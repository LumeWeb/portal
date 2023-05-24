package tusstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/model"
	"git.lumeweb.com/LumeWeb/portal/shared"
	"github.com/golang-queue/queue"
	"github.com/tus/tusd/pkg/handler"
	"go.uber.org/zap"
	"io"
	"os"
	"path/filepath"
)

var defaultFilePerm = os.FileMode(0664)

type DbFileStore struct {
	// Relative or absolute path to store files in. DbFileStore does not check
	// whether the path exists, use os.MkdirAll in this case on your own.
	Path string
}

func (store DbFileStore) UseIn(composer *handler.StoreComposer) {
	composer.UseCore(store)
	composer.UseTerminater(store)
	composer.UseConcater(store)
	composer.UseLengthDeferrer(store)
}

func (store DbFileStore) NewUpload(ctx context.Context, info handler.FileInfo) (handler.Upload, error) {
	if info.ID == "" {
		info.ID = uid()
	}
	binPath := store.binPath(info.ID)
	info.Storage = map[string]string{
		"Type": "dbstore",
		"Path": binPath,
	}

	// Create binary file with no content
	file, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("upload directory does not exist: %s", store.Path)
		}
		return nil, err
	}

	err = file.Close()
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	upload := &fileUpload{
		info:    info,
		binPath: binPath,
		hash:    info.MetaData["blake3-hash"],
	}

	// writeInfo creates the file by itself if necessary
	err = upload.writeInfo()
	if err != nil {
		return nil, err
	}

	return upload, nil
}

func (store DbFileStore) GetUpload(ctx context.Context, id string) (handler.Upload, error) {
	info := handler.FileInfo{
		ID: id,
	}

	fUpload := &fileUpload{info: info}

	record, is404, err := fUpload.getInfo()
	if err != nil {
		if is404 {
			// Interpret os.ErrNotExist as 404 Not Found
			err = handler.ErrNotFound
		}
		return nil, err
	}

	if err := json.Unmarshal([]byte(record.Info), &info); err != nil {
		logger.Get().Error("fail to parse upload meta", zap.Error(err))
		return nil, err
	}

	fUpload.info = info

	fUpload.hash = record.Hash
	binPath := store.binPath(id)
	stat, err := os.Stat(binPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Interpret os.ErrNotExist as 404 Not Found
			err = handler.ErrNotFound
		}
		return nil, err
	}

	info.Offset = stat.Size()

	fUpload.binPath = binPath

	return fUpload, nil
}

func (store DbFileStore) AsTerminatableUpload(upload handler.Upload) handler.TerminatableUpload {
	return upload.(*fileUpload)
}

func (store DbFileStore) AsLengthDeclarableUpload(upload handler.Upload) handler.LengthDeclarableUpload {
	return upload.(*fileUpload)
}

func (store DbFileStore) AsConcatableUpload(upload handler.Upload) handler.ConcatableUpload {
	return upload.(*fileUpload)
}

// binPath returns the path to the file storing the binary data.
func (store DbFileStore) binPath(id string) string {
	return filepath.Join(store.Path, id)
}

type fileUpload struct {
	// info stores the current information about the upload
	info handler.FileInfo
	// binPath is the path to the binary file (which has no extension)
	binPath string
	hash    string
}

func (upload *fileUpload) GetInfo(ctx context.Context) (handler.FileInfo, error) {
	return upload.info, nil
}

func (upload *fileUpload) WriteChunk(ctx context.Context, offset int64, src io.Reader) (int64, error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	n, err := io.Copy(file, src)

	upload.info.Offset += n

	err = upload.writeInfo()
	if err != nil {
		return 0, err
	}

	return n, err
}

func (upload *fileUpload) GetReader(ctx context.Context) (io.Reader, error) {
	return os.Open(upload.binPath)
}

func (upload *fileUpload) Terminate(ctx context.Context) error {
	tusUpload := &model.Tus{
		UploadID: upload.info.ID,
	}

	ret := db.Get().Where(&tusUpload).Delete(&tusUpload)

	if ret.Error != nil {
		logger.Get().Error("failed to delete tus entry", zap.Error(ret.Error))
	}

	if err := os.Remove(upload.binPath); err != nil {
		return err
	}

	return nil
}

func (upload *fileUpload) ConcatUploads(ctx context.Context, uploads []handler.Upload) (err error) {
	file, err := os.OpenFile(upload.binPath, os.O_WRONLY|os.O_APPEND, defaultFilePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, partialUpload := range uploads {
		fileUpload := partialUpload.(*fileUpload)

		src, err := os.Open(fileUpload.binPath)
		if err != nil {
			return err
		}

		if _, err := io.Copy(file, src); err != nil {
			return err
		}
	}

	return
}

func (upload *fileUpload) DeclareLength(ctx context.Context, length int64) error {
	upload.info.Size = length
	upload.info.SizeIsDeferred = false
	return upload.writeInfo()
}

// writeInfo updates the entire information. Everything will be overwritten.
func (upload *fileUpload) writeInfo() error {
	data, err := json.Marshal(upload.info)
	if err != nil {
		return err
	}

	tusRecord, is404, err := upload.getInfo()

	if err != nil && !is404 {
		return err
	}

	if tusRecord != nil {
		tusRecord.Info = string(data)
		if ret := db.Get().Update("info", &tusRecord); ret.Error != nil {
			logger.Get().Error("failed to update tus entry", zap.Error(ret.Error))

			return ret.Error
		}
	}

	tusRecord = &model.Tus{UploadID: upload.info.ID, Hash: upload.hash, Info: string(data)}

	if ret := db.Get().Create(&tusRecord); ret.Error != nil {
		logger.Get().Error("failed to create tus entry", zap.Error(ret.Error))

		return ret.Error
	}

	return nil
}

func (upload *fileUpload) getInfo() (*model.Tus, bool, error) {
	var tusRecord model.Tus

	result := db.Get().Where(&model.Tus{UploadID: upload.info.ID}).First(&tusRecord)

	if result.Error != nil && result.Error.Error() != "record not found" {
		logger.Get().Error("failed to query tus entry", zap.Error(result.Error))
		return nil, false, result.Error
	}

	if result.Error != nil {
		return nil, true, result.Error
	}

	return &tusRecord, false, nil
}

func (upload *fileUpload) FinishUpload(ctx context.Context) error {
	if err := getQueue().QueueTask(func(ctx context.Context) error {
		upload, err := getStore().GetUpload(nil, upload.info.ID)
		if err != nil {
			logger.Get().Error("failed to query tus upload", zap.Error(err))
			return err
		}
		return shared.GetTusWorker()(&upload)
	}); err != nil {
		logger.Get().Error("failed to queue tus upload", zap.Error(err))
		return err
	}

	return nil
}

func uid() string {
	id := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		// This is probably an appropriate way to handle errors from our source
		// for random bits.
		panic(err)
	}
	return hex.EncodeToString(id)
}
func getQueue() *queue.Queue {
	ret := shared.GetTusQueue()
	return (*ret).(*queue.Queue)
}

func getStore() *DbFileStore {
	ret := shared.GetTusStore()
	return (*ret).(*DbFileStore)
}
