package models

import "gorm.io/gorm"

func init() {
	registerModel(&TusUpload{})

}

type TusUpload struct {
	gorm.Model
	Hash       []byte `gorm:"type:binary(32);"`
	MimeType   string
	UploadID   string `gorm:"type:varchar(500);uniqueIndex"`
	UploaderID uint
	UploaderIP string
	Uploader   User `gorm:"foreignKey:UploaderID"`
	Protocol   string
	Completed  bool
}
