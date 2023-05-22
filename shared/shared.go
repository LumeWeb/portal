package shared

import (
	"git.lumeweb.com/LumeWeb/portal/tusstore"
	"github.com/golang-queue/queue"
	tusd "github.com/tus/tusd/pkg/handler"
	_ "go.uber.org/zap"
)

var tusQueue *queue.Queue
var tusStore *tusstore.DbFileStore
var tusComposer *tusd.StoreComposer

func SetTusQueue(q *queue.Queue) {
	tusQueue = q
}

func GetTusQueue() *queue.Queue {
	return tusQueue
}

func SetTusStore(s *tusstore.DbFileStore) {
	tusStore = s
}

func GetTusStore() *tusstore.DbFileStore {
	return tusStore
}

func SetTusComposer(c *tusd.StoreComposer) {
	tusComposer = c
}

func GetTusComposer() *tusd.StoreComposer {
	return tusComposer
}
