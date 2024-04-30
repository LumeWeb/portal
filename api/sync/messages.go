package sync

type LogKeyResponse struct {
	Key string `json:"key"`
}

type ObjectImportRequest struct {
	Object string `json:"object"`
}
