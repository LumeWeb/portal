package service

import (
	"context"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

var _ core.DNSLinkService = (*DNSLinkServiceDefault)(nil)

type DNSLinkServiceDefault struct {
	ctx      *core.Context
	config   config.Manager
	db       *gorm.DB
	user     core.UserService
	metadata core.MetadataService
	pin      core.PinService
}

func NewDNSLinkService(ctx *core.Context) *DNSLinkServiceDefault {
	dnslinkService := &DNSLinkServiceDefault{
		ctx:      ctx,
		config:   ctx.Config(),
		db:       ctx.DB(),
		user:     ctx.Services().User(),
		metadata: ctx.Services().Metadata(),
		pin:      ctx.Services().Pin(),
	}
	ctx.RegisterService(dnslinkService)

	return dnslinkService
}

func (p DNSLinkServiceDefault) DNSLinkExists(hash []byte) (bool, *models.DNSLink, error) {
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
