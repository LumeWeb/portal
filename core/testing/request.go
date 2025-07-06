package testing

import (
	mh "github.com/multiformats/go-multihash"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

// RequestBuilder is a helper for creating models.Request instances with a fluent API and reasonable defaults.
type RequestBuilder struct {
	request *models.Request
}

// NewRequest creates a new RequestBuilder instance with reasonable defaults.
func NewRequest(proto string, op core.OperationType) *RequestBuilder {
	return &RequestBuilder{
		request: &models.Request{
			Protocol:  proto,
			Operation: string(op),
			Status:    models.RequestStatusPending,
			CIDType:   0,
		},
	}
}

// WithID sets the ID of the request.
func (rb *RequestBuilder) WithID(id uint) *RequestBuilder {
	rb.request.ID = id
	return rb
}

// WithProtocol sets the Protocol of the request.
func (rb *RequestBuilder) WithProtocol(protocol string) *RequestBuilder {
	rb.request.Protocol = protocol
	return rb
}

// WithOperation sets the Operation of the request.
func (rb *RequestBuilder) WithOperation(operation string) *RequestBuilder {
	rb.request.Operation = operation
	return rb
}

// WithStatus sets the Status of the request.
func (rb *RequestBuilder) WithStatus(status models.RequestStatusType) *RequestBuilder {
	rb.request.Status = status
	return rb
}

// WithMultihash sets the Hash of the request using a raw multihash.
func (rb *RequestBuilder) WithStorageHash(hash core.StorageHash) *RequestBuilder {
	rb.request.Hash = hash.Multihash()
	return rb
}

// WithStorageHash sets the Hash of the request.
func (rb *RequestBuilder) WithMultihash(hash mh.Multihash) *RequestBuilder {
	rb.request.Hash = hash
	return rb
}

// WithCIDType sets the CIDType of the request.
func (rb *RequestBuilder) WithCIDType(cidType uint64) *RequestBuilder {
	rb.request.CIDType = cidType
	return rb
}

// WithUserID sets the UserID of the request.
func (rb *RequestBuilder) WithUserID(userID uint) *RequestBuilder {
	rb.request.UserID = &userID
	return rb
}

// WithMetadata sets the Metadata of the request.
func (rb *RequestBuilder) WithMetadata(metadata []byte) *RequestBuilder {
	rb.request.Metadata = metadata
	return rb
}

// Build creates the final models.Request instance.
func (rb *RequestBuilder) Build() *models.Request {
	return rb.request
}
