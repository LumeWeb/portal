package shared

import (
	tusd "github.com/tus/tusd/pkg/handler"
	_ "go.uber.org/zap"
)

type TusFunc func(upload *tusd.Upload) error

var tusQueue *interface{}
var tusStore *interface{}
var tusComposer *interface{}
var tusWorker TusFunc

func SetTusQueue(q interface{}) {
	tusQueue = &q
}

func GetTusQueue() *interface{} {
	return tusQueue
}

func SetTusStore(s interface{}) {
	tusStore = &s
}

func GetTusStore() *interface{} {
	return tusStore
}

func SetTusComposer(c interface{}) {
	tusComposer = &c
}

func GetTusComposer() *interface{} {
	return tusComposer
}

func SetTusWorker(w TusFunc) {
	tusWorker = w
}

func GetTusWorker() TusFunc {
	return tusWorker
}
