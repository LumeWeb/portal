package models

import "gorm.io/gorm"

func init() {
	registerModel(&TusUpload{})

}

type TusUpload struct {
	gorm.Model
	Hash       []byte `gorm:"type:binary(32);uniqueIndex:idx_hash_deleted"`
	MimeType   string
	UploadID   string `gorm:"uniqueIndex"`
	UploaderID uint
	UploaderIP string
	Uploader   User `gorm:"foreignKey:UploaderID"`
	Protocol   string
	Completed  bool
	DeletedAt  gorm.DeletedAt `gorm:"uniqueIndex:idx_hash_deleted"`
}
