package tus

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"git.lumeweb.com/LumeWeb/portal/cid"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/model"
	"git.lumeweb.com/LumeWeb/portal/service/files"
	"git.lumeweb.com/LumeWeb/portal/shared"
	"github.com/golang-queue/queue"
	tusd "github.com/tus/tusd/pkg/handler"
	"github.com/tus/tusd/pkg/memorylocker"
	"go.uber.org/zap"
	"io"
	"log"
)

const TUS_API_PATH = "/files/tus"

const HASH_META_HEADER = "blake3-hash"

func Init() *tusd.Handler {
	store := &filestore.FileStore{
		Path: "/tmp",
	}

	shared.SetTusStore(store)

	composer := tusd.NewStoreComposer()
	composer.UseCore(store)
	composer.UseConcater(store)
	composer.UseLocker(memorylocker.New())
	composer.UseTerminater(store)
	shared.SetTusComposer(composer)

	handler, err := tusd.NewHandler(tusd.Config{
		BasePath:      "/api/v1" + TUS_API_PATH,
		StoreComposer: composer,
		PreUploadCreateCallback: func(hook tusd.HookEvent) error {
			hash := hook.Upload.MetaData[HASH_META_HEADER]

			if len(hash) == 0 {
				msg := "missing blake3-hash metadata"
				logger.Get().Debug(msg)
				return errors.New(msg)
			}

			var upload model.Upload
			result := db.Get().Where(&model.Upload{Hash: hash}).First(&upload)
			if (result.Error != nil && result.Error.Error() != "record not found") || result.RowsAffected > 0 {
				hashBytes, err := hex.DecodeString(hash)
				if err != nil {
					logger.Get().Debug("invalid hash", zap.Error(err))
					return err
				}

				cidString, err := cid.Encode(hashBytes, uint64(hook.Upload.Size))

				if err != nil {
					logger.Get().Debug("failed to create cid", zap.Error(err))
					return err
				}

				resp, err := json.Marshal(UploadResponse{Cid: cidString})

				if err != nil {
					logger.Get().Error("failed to create response", zap.Error(err))
					return err
				}

				return tusd.NewHTTPError(errors.New(string(resp)), 304)
			}

			return nil
		},
		PreFinishResponseCallback: func(hook tusd.HookEvent) error {
			tusEntry := &model.Tus{
				UploadID: hook.Upload.ID,
				Hash:     hook.Upload.MetaData[HASH_META_HEADER],
			}

			if err := db.Get().Create(tusEntry).Error; err != nil {
				logger.Get().Error("failed to create tus entry", zap.Error(err))
				return err
			}

			if err := shared.GetTusQueue().QueueTask(func(ctx context.Context) error {
				upload, err := store.GetUpload(nil, hook.Upload.ID)
				if err != nil {
					logger.Get().Error("failed to query tus upload", zap.Error(err))
					return err
				}
				return tusWorker(&upload)
			}); err != nil {
				logger.Get().Error("failed to queue tus upload", zap.Error(err))
				return err
			}

			return nil
		},
	})
	if err != nil {
		panic(err)
	}

	shared.SetTusQueue(queue.NewPool(5))

	go tusStartup()

	return handler
}

func tusStartup() {
	result := map[int]model.Tus{}
	db.Get().Table("tus").Take(&result)

	tusQueue := shared.GetTusQueue()
	store := shared.GetTusStore()

	for _, item := range result {
		if err := tusQueue.QueueTask(func(ctx context.Context) error {
			upload, err := store.GetUpload(nil, item.UploadID)
			if err != nil {
				logger.Get().Error("failed to query tus upload", zap.Error(err))
				return err
			}
			return tusWorker(&upload)
		}); err != nil {
			log.Print(err)
		}
	}
}

func tusWorker(upload *tusd.Upload) error {
	info, err := (*upload).GetInfo(context.Background())
	if err != nil {
		logger.Get().Error("failed to query tus upload metadata", zap.Error(err))
		return err
	}
	file, err := (*upload).GetReader(context.Background())
	if err != nil {
		logger.Get().Error("failed reading upload", zap.Error(err))
		return err
	}

	hashHex := info.MetaData[HASH_META_HEADER]

	hashBytes, err := hex.DecodeString(hashHex)

	if err != nil {
		logger.Get().Error("failed decoding hash", zap.Error(err))
		tErr := terminateUpload(*upload)

		if tErr != nil {
			return tErr
		}
		return err
	}

	_, err = files.Upload(file.(io.ReadSeeker), info.Size, hashBytes)
	tErr := terminateUpload(*upload)

	if tErr != nil {
		return tErr
	}

	if err != nil {
		return err
	}

	return nil
}

func terminateUpload(upload tusd.Upload) error {
	info, _ := upload.GetInfo(context.Background())
	err := shared.GetTusComposer().Terminater.AsTerminatableUpload(upload).Terminate(context.Background())

	if err != nil {
		logger.Get().Error("failed deleting tus upload", zap.Error(err))
	}

	tusUpload := &model.Tus{UploadID: info.ID}
	ret := db.Get().Where(tusUpload).First(&tusUpload)

	if ret.Error != nil && ret.Error.Error() != "record not found" {
		logger.Get().Error("failed fetching tus entry", zap.Error(err))
		err = ret.Error
	}

	err1 := db.Get().Where(&tusUpload).Delete(&tusUpload)

	_ = err1

	if err != nil {
		return err
	}

	return nil
}

type UploadResponse struct {
	Cid string `json:"cid"`
}
