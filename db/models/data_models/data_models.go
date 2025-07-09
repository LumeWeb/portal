package data_models

type RequestDataModel interface {
	TableName() string
	Validate() error
	NewInstance() RequestDataModel
	SetRequestID(id uint)
	GetRequestID() uint
	SetRequest(req any)
}

type PinDataModel interface {
	TableName() string
	Validate() error
	NewInstance() PinDataModel
	SetPinID(id uint)
	GetPinID() uint
	SetPin(req any)
}
