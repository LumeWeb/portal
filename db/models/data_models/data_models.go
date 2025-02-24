package data_models

type RequestDataModel interface {
	TableName() string
	Validate() error
	NewInstance() RequestDataModel
	SetRequestID(id uint)
	GetRequestID() uint
}

type PinDataModel interface {
	TableName() string
	Validate() error
	NewInstance() PinDataModel
	SetPinID(id uint)
	GetPinID() uint
}
