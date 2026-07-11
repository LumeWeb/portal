package indexd

import (
	"encoding/json"

	sdk "go.sia.tech/siastorage"
)

// ObjectMetadata is embedded in each SDK object before pinning. It attributes
// the object to its portal protocol and key, enabling the indexer to be used
// as a source of truth for object provenance.
type ObjectMetadata struct {
	Protocol  string `json:"protocol"`
	ObjectKey string `json:"objectKey"`
}

// SetObjectMetadata sets the protocol/key metadata on an SDK object before
// pinning. PinObject calls Seal(), which encrypts the metadata with the app
// key and persists it alongside the object's slabs in the indexer.
func SetObjectMetadata(obj *sdk.Object, protocol, objectKey string) error {
	meta := ObjectMetadata{
		Protocol:  protocol,
		ObjectKey: objectKey,
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	obj.UpdateMetadata(data)
	return nil
}
