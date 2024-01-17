package s5

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
	Email          string `json:"email"`
	QuotaExceeded  bool   `json:"quotaExceeded"`
	EmailConfirmed bool   `json:"emailConfirmed"`
	IsRestricted   bool   `json:"isRestricted"`
	Tier           uint8  `json:"tier"`
}

type AccountStatsResponse struct {
	AccountInfoResponse
	Stats AccountStats `json:"stats"`
}

type AccountStats struct {
	Total AccountStatsTotal `json:"total"`
}

type AccountStatsTotal struct {
	UsedStorage uint64 `json:"usedStorage"`
}
