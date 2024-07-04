package service

import (
	"context"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

var _ core.DNSLinkService = (*DNSLinkServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.DNSLINK_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewDNSLinkService()
		},
		Depends: []string{core.USER_SERVICE, core.METADATA_SERVICE, core.PIN_SERVICE},
	})
}

type DNSLinkServiceDefault struct {
	ctx      core.Context
	config   config.Manager
	db       *gorm.DB
	user     core.UserService
	metadata core.MetadataService
	pin      core.PinService
}

func NewDNSLinkService() (*DNSLinkServiceDefault, []core.ContextBuilderOption, error) {
	dnslinkService := &DNSLinkServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			dnslinkService.ctx = ctx
			dnslinkService.config = ctx.Config()
			dnslinkService.db = ctx.DB()
			dnslinkService.user = ctx.Service(core.USER_SERVICE).(core.UserService)
			dnslinkService.metadata = ctx.Service(core.METADATA_SERVICE).(core.MetadataService)
			dnslinkService.pin = ctx.Service(core.PIN_SERVICE).(core.PinService)
			return nil
		}),
	)

	return dnslinkService, opts, nil
}

func (p DNSLinkServiceDefault) DNSLinkExists(hash core.StorageHash) (bool, *models.DNSLink, error) {
	upload, err := p.metadata.GetUpload(context.Background(), hash)
	if err != nil {
		return false, nil, err
	}

	exists, model, err := p.user.Exists(&models.DNSLink{}, map[string]interface{}{"upload_id": upload.ID})
	if !exists || err != nil {
		return false, nil, err
	}

	pinned, err := p.pin.UploadPinnedGlobal(hash)
	if err != nil {
		return false, nil, err
	}

	if !pinned {
		return false, nil, nil
	}

	return true, model.(*models.DNSLink), nil
}
