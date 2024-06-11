package core

type SyncProtocol interface {
	Name() string
	EncodeFileName([]byte) string
	ValidIdentifier(string) bool
	HashFromIdentifier(string) ([]byte, error)
	StorageProtocol() StorageProtocol
}
type SyncService interface {
	Update(upload UploadMetadata) error
	LogKey() []byte
	Import(object string, uploaderID uint64) error
	Enabled() bool
}
