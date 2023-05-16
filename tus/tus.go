package tus

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"git.lumeweb.com/LumeWeb/portal/cid"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/model"
	"git.lumeweb.com/LumeWeb/portal/service/files"
	"git.lumeweb.com/LumeWeb/portal/shared"
	"github.com/golang-queue/queue"
	"github.com/tus/tusd/pkg/filestore"
	tusd "github.com/tus/tusd/pkg/handler"
	"github.com/tus/tusd/pkg/memorylocker"
	"log"
	"os"
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

	handler, err := tusd.NewHandler(tusd.Config{
		BasePath:      "/api/v1" + TUS_API_PATH,
		StoreComposer: composer,
		PreUploadCreateCallback: func(hook tusd.HookEvent) error {
			hash := hook.Upload.MetaData[HASH_META_HEADER]

			if len(hash) == 0 {
				return errors.New("missing blake3-hash metadata")
			}

			var upload model.Upload
			result := db.Get().Where(&model.Upload{Hash: hash}).First(&upload)
			if (result.Error != nil && result.Error.Error() != "record not found") || result.RowsAffected > 0 {
				hashBytes, err := hex.DecodeString(hash)
				if err != nil {
					return err
				}

				cidString, err := cid.Encode(hashBytes, uint64(hook.Upload.Size))

				if err != nil {
					return err
				}

				resp, err := json.Marshal(UploadResponse{Cid: cidString})

				if err != nil {
					return err
				}

				return tusd.NewHTTPError(errors.New(string(resp)), 304)
			}

			return nil
		},
		PreFinishResponseCallback: func(hook tusd.HookEvent) error {
			tusEntry := &model.Tus{
				Id:   hook.Upload.ID,
				Hash: hook.Upload.MetaData[HASH_META_HEADER],
			}

			if err := db.Get().Create(tusEntry).Error; err != nil {
				return err
			}

			if err := shared.GetTusQueue().QueueTask(func(ctx context.Context) error {
				upload, err := store.GetUpload(nil, hook.Upload.ID)
				if err != nil {
					return err
				}
				return tusWorker(&upload)
			}); err != nil {
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
				return err
			}
			return tusWorker(&upload)
		}); err != nil {
			log.Print(err)
		}
	}
}

func tusWorker(upload *tusd.Upload) error {
	info, err := (*upload).GetInfo(nil)
	if err != nil {
		log.Print(err)
		return err
	}
	file, err := (*upload).GetReader(nil)
	if err != nil {
		log.Print(err)
		return err
	}

	_, err = files.Upload(nil, file.(*os.File))
	if err != nil {
		log.Print(err)
		return err
	}

	hash := info.MetaData[HASH_META_HEADER]

	var tusUpload model.Tus
	ret := db.Get().Where(&model.Tus{Hash: hash}).First(&tusUpload)
	if ret.Error != nil && ret.Error.Error() != "record not found" {
		log.Print(ret.Error)
		return err
	}

	ret = db.Get().Delete(&tusUpload)

	err = shared.GetTusComposer().Terminater.AsTerminatableUpload(*upload).Terminate(context.Background())

	if err != nil {
		log.Print(err)
		return err
	}

	return nil
}

type UploadResponse struct {
	Cid string `json:"cid"`
}
