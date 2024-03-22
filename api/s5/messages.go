package s5

import (
	"time"

	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"github.com/vmihailenco/msgpack/v5"
)

var (
	_ msgpack.CustomEncoder = (*AccountPinBinaryResponse)(nil)
)

type AccountRegisterRequest struct {
	Pubkey    string `json:"pubkey"`
	Response  string `json:"response"`
	Signature string `json:"signature"`
	Email     string `json:"email,omitempty"`
}

type SmallUploadResponse struct {
	CID string `json:"cid"`
}
type AccountRegisterChallengeResponse struct {
	Challenge string `json:"challenge"`
}

type AccountLoginRequest struct {
	Pubkey    string `json:"pubkey"`
	Response  string `json:"response"`
	Signature string `json:"signature"`
}
type AccountLoginChallengeResponse struct {
	Challenge string `json:"challenge"`
}
type AccountInfoResponse struct {
	Email          string      `json:"email"`
	QuotaExceeded  bool        `json:"quotaExceeded"`
	EmailConfirmed bool        `json:"emailConfirmed"`
	IsRestricted   bool        `json:"isRestricted"`
	Tier           AccountTier `json:"tier"`
}

type AccountStatsResponse struct {
	AccountInfoResponse
	Stats AccountStats `json:"stats"`
}

type AccountTier struct {
	Id              uint64        `json:"id"`
	Name            string        `json:"name"`
	UploadBandwidth uint64        `json:"uploadBandwidth"`
	StorageLimit    uint64        `json:"storageLimit"`
	Scopes          []interface{} `json:"scopes"`
}

type AccountStats struct {
	Total AccountStatsTotal `json:"total"`
}

type AccountStatsTotal struct {
	UsedStorage uint64 `json:"usedStorage"`
}
type AppUploadResponse struct {
	CID string `json:"cid"`
}
type RegistryQueryResponse struct {
	Pk        string `json:"pk"`
	Revision  uint64 `json:"revision"`
	Data      string `json:"data"`
	Signature string `json:"signature"`
}

type RegistrySetRequest struct {
	Pk        string `json:"pk"`
	Revision  uint64 `json:"revision"`
	Data      string `json:"data"`
	Signature string `json:"signature"`
}

type DebugStorageLocation struct {
	Type   int      `json:"type"`
	Parts  []string `json:"parts"`
	Expiry int64    `json:"expiry"`
	NodeId string   `json:"nodeId"`
	Score  float64  `json:"score"`
}

type DebugStorageLocationsResponse struct {
	Locations []DebugStorageLocation `json:"locations"`
}

type AccountPinBinaryResponse struct {
	Pins   []models.Pin
	Cursor uint64
}

func (a AccountPinBinaryResponse) EncodeMsgpack(enc *msgpack.Encoder) error {
	err := enc.EncodeInt(0)
	if err != nil {
		return err
	}

	err = enc.EncodeInt(int64(a.Cursor))
	if err != nil {
		return err
	}

	err = enc.EncodeArrayLen(len(a.Pins))
	if err != nil {
		return err
	}

	for _, pin := range a.Pins {
		err = enc.EncodeBytes(encoding.MultihashFromBytes(pin.Upload.Hash, types.HashTypeBlake3).FullBytes())
		if err != nil {
			return err
		}
	}

	return nil
}

type AccountPinResponse struct {
	Pins []AccountPin `json:"pins"`
}

type AccountPin struct {
	Hash     string    `json:"hash"`
	Size     uint64    `json:"size"`
	PinnedAt time.Time `json:"pinned_at"`
	MimeType string    `json:"mime_type"`
}

type AccountPinStatusResponse struct {
	Status   models.ImportStatus `json:"status"`
	Progress float64             `json:"progress"`
}
