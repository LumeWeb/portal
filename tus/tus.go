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
	"git.lumeweb.com/LumeWeb/portal/tusstore"
	"github.com/golang-queue/queue"
	tusd "github.com/tus/tusd/pkg/handler"
	"github.com/tus/tusd/pkg/memorylocker"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
	"io"
	"strconv"
)

const TUS_API_PATH = "/files/tus"

const HASH_META_HEADER = "hash"

func Init() *tusd.Handler {
	store := &tusstore.DbFileStore{
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
				msg := "missing hash metadata"
				logger.Get().Debug(msg)
				return errors.New(msg)
			}

			var upload model.Upload
			result := db.Get().Where(&model.Upload{Hash: hash}).First(&upload)
			if (result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound)) || result.RowsAffected > 0 {
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
	})
	if err != nil {
		panic(err)
	}

	pool := queue.NewPool(5)

	shared.SetTusQueue(pool)
	shared.SetTusWorker(tusWorker)

	go tusStartup()

	return handler
}

func tusStartup() {
	tusQueue := getQueue()
	store := getStore()

	rows, err := db.Get().Model(&model.Tus{}).Rows()

	if err != nil {
		logger.Get().Error("failed to query tus uploads", zap.Error(err))
	}

	defer rows.Close()

	processedHashes := make([]string, 0)

	for rows.Next() {
		var tusUpload model.Tus
		err := db.Get().ScanRows(rows, &tusUpload)
		if err != nil {
			logger.Get().Error("failed to scan tus records", zap.Error(err))
			return
		}

		upload, err := store.GetUpload(nil, tusUpload.UploadID)
		if err != nil {
			logger.Get().Error("failed to query tus upload", zap.Error(err))
			db.Get().Delete(&tusUpload)
			continue
		}

		if slices.Contains(processedHashes, tusUpload.Hash) {
			err := terminateUpload(upload)
			if err != nil {
				logger.Get().Error("failed to terminate tus upload", zap.Error(err))
			}
			continue
		}

		if err := tusQueue.QueueTask(func(ctx context.Context) error {
			return tusWorker(&upload)
		}); err != nil {
			logger.Get().Error("failed to queue tus upload", zap.Error(err))
		} else {
			processedHashes = append(processedHashes, tusUpload.Hash)
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

	uploader, _ := strconv.Atoi(info.Storage["uploader"])

	newUpload, err := files.Upload(file.(io.ReadSeeker), info.Size, hashBytes, uint(uploader))
	tErr := terminateUpload(*upload)

	if tErr != nil {
		return tErr
	}

	if err != nil {
		return err
	}

	err = files.Pin(newUpload.Hash, newUpload.AccountID)
	if err != nil {
		return err
	}

	return nil
}

func terminateUpload(upload tusd.Upload) error {
	err := getComposer().Terminater.AsTerminatableUpload(upload).Terminate(context.Background())

	if err != nil {
		logger.Get().Error("failed deleting tus upload", zap.Error(err))
	}

	if err != nil {
		return err
	}

	return nil
}

type UploadResponse struct {
	Cid string `json:"cid"`
}

func getQueue() *queue.Queue {
	ret := shared.GetTusQueue()
	return (*ret).(*queue.Queue)
}

func getStore() *tusstore.DbFileStore {
	ret := shared.GetTusStore()
	return (*ret).(*tusstore.DbFileStore)
}
func getComposer() *tusd.StoreComposer {
	ret := shared.GetTusComposer()
	return (*ret).(*tusd.StoreComposer)
}
