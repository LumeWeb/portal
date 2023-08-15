package dnslink

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/cid"
	"git.lumeweb.com/LumeWeb/portal/controller"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/model"
	"git.lumeweb.com/LumeWeb/portal/service/files"
	dnslink "github.com/dnslink-std/go"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/context"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/zap"
	"io"
	"path/filepath"
	"strings"
)

var (
	ErrFailedReadAppManifest = errors.New("failed to read app manifest")
	ErrInvalidAppManifest    = errors.New("invalid app manifest")
)

type CID string
type ExtraMetadata map[string]interface{}

type WebAppMetadata struct {
	Schema        string                 `msgpack:"$schema,omitempty"`
	Type          string                 `msgpack:"type"`
	Name          string                 `msgpack:"name,omitempty"`
	TryFiles      []string               `msgpack:"tryFiles,omitempty"`
	ErrorPages    map[string]string      `msgpack:"errorPages,omitempty"`
	Paths         map[string]PathContent `msgpack:"paths"`
	ExtraMetadata ExtraMetadata          `msgpack:"extraMetadata,omitempty"`
}

type PathContent struct {
	CID         CID    `msgpack:"cid"`
	ContentType string `msgpack:"contentType,omitempty"`
}

func Handler(ctx *context.Context) {
	record := model.Dnslink{}

	domain := ctx.Request().Host

	if err := db.Get().Model(&model.Dnslink{Domain: domain}).First(&record).Error; err != nil {
		ctx.StopWithStatus(iris.StatusNotFound)
		return
	}
	ret, err := dnslink.Resolve(domain)
	if err != nil {
		switch e := err.(type) {
		default:
			ctx.StopWithStatus(iris.StatusInternalServerError)
			return
		case dnslink.DNSRCodeError:
			if e.DNSRCode == 3 {
				ctx.StopWithStatus(iris.StatusNotFound)
				return
			}
		}
	}

	if ret.Links["sia"] == nil || len(ret.Links["sia"]) == 0 {
		ctx.StopWithStatus(iris.StatusNotFound)
		return
	}

	appManifest := ret.Links["sia"][0]

	decodedCid, valid := controller.ValidateCid(appManifest.Identifier, true, ctx)
	if !valid {
		return
	}

	manifest := fetchManifest(ctx, decodedCid)
	if manifest == nil {
		return
	}

	path := ctx.Path()

	if strings.HasSuffix(path, "/") || filepath.Ext(path) == "" {
		var directoryIndex *PathContent
		for _, indexFile := range manifest.TryFiles {
			path, exists := manifest.Paths[indexFile]

			if !exists {
				continue
			}

			_, err := cid.Valid(string(manifest.Paths[indexFile].CID))

			if err != nil {
				continue
			}

			cidObject, _ := cid.Decode(string(path.CID))
			hashHex := cidObject.StringHash()

			status := files.Status(hashHex)

			if status == files.STATUS_NOT_FOUND {
				continue
			}
			if status == files.STATUS_UPLOADED {
				directoryIndex = &path
				break
			}
		}

		if directoryIndex == nil {
			ctx.StopWithStatus(iris.StatusNotFound)
			return
		}

		file, err := fetchFile(directoryIndex)
		if maybeHandleFileError(err, ctx) {
			return
		}
		ctx.Header("Content-Type", directoryIndex.ContentType)
		streamFile(file, ctx)
		return
	}

	path = strings.TrimLeft(path, "/")

	requestedPath, exists := manifest.Paths[path]

	if !exists {
		ctx.StopWithStatus(iris.StatusNotFound)
		return
	}

	file, err := fetchFile(&requestedPath)
	if maybeHandleFileError(err, ctx) {
		return
	}
	ctx.Header("Content-Type", requestedPath.ContentType)
	streamFile(file, ctx)
}

func maybeHandleFileError(err error, ctx *context.Context) bool {
	if err != nil {
		if err == files.ErrInvalidFile {
			controller.SendError(ctx, err, iris.StatusNotFound)
			return true
		}
		controller.SendError(ctx, err, iris.StatusInternalServerError)
	}

	return err != nil
}

func streamFile(stream io.Reader, ctx *context.Context) {
	err := controller.PassThroughStream(stream, ctx)
	if err != controller.ErrStreamDone && controller.InternalError(ctx, err) {
		logger.Get().Debug("failed streaming file", zap.Error(err))
	}
}

func fetchFile(path *PathContent) (io.Reader, error) {
	_, err := cid.Valid(string(path.CID))

	if err != nil {
		return nil, err
	}

	cidObject, _ := cid.Decode(string(path.CID))
	hashHex := cidObject.StringHash()

	status := files.Status(hashHex)

	if status == files.STATUS_NOT_FOUND {
		return nil, errors.New("cid not found")
	}
	if status == files.STATUS_UPLOADED {
		stream, err := files.Download(hashHex)

		if err != nil {
			return nil, err
		}

		return stream, nil
	}

	return nil, errors.New("cid not found")
}

func fetchManifest(ctx iris.Context, hash string) *WebAppMetadata {
	stream, err := files.Download(hash)

	if err != nil {
		if errors.Is(err, files.ErrInvalidFile) {
			controller.SendError(ctx, err, iris.StatusNotFound)
			return nil
		}
		controller.SendError(ctx, err, iris.StatusInternalServerError)
	}
	var metadata WebAppMetadata

	data, err := io.ReadAll(stream)

	if err != nil {
		logger.Get().Debug(ErrFailedReadAppManifest.Error(), zap.Error(err))
		controller.SendError(ctx, ErrFailedReadAppManifest, iris.StatusInternalServerError)
		return nil
	}

	err = msgpack.Unmarshal(data, &metadata)
	if err != nil {
		logger.Get().Debug(ErrFailedReadAppManifest.Error(), zap.Error(err))
		controller.SendError(ctx, ErrFailedReadAppManifest, iris.StatusInternalServerError)
		return nil
	}

	if metadata.Type != "web_app" {
		logger.Get().Debug(ErrInvalidAppManifest.Error())
		controller.SendError(ctx, ErrInvalidAppManifest, iris.StatusInternalServerError)
		return nil
	}

	return &metadata
}
