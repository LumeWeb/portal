package models

import (
	"reflect"

	"github.com/samber/lo"
)

var registered []interface{}

func registerModel(model interface{}) {
	// Check if the model type is already registered
	modelType := reflect.TypeOf(model)
	isDuplicate := lo.ContainsBy(registered, func(item interface{}) bool {
		return reflect.TypeOf(item) == modelType
	})

	if !isDuplicate {
		registered = append(registered, model)
	}
}

func GetModels() []interface{} {
	return registered
}
