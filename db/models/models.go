package models

var registered []interface{}

func registerModel(model interface{}) {
	registered = append(registered, model)
}

func GetModels() []interface{} {
	return registered
}
