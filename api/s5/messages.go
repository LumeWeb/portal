package s5

type AccountRegisterRequest struct {
	Pubkey    string `json:"pubkey"`
	Response  string `json:"response"`
	Signature string `json:"signature"`
	Email     string `json:"email"`
}

type SmallUploadResponse struct {
	CID string `json:"cid"`
}
type AccountRegisterChallengeResponse struct {
	Challenge string `json:"challenge"`
}
