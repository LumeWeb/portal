package shared

import (
	"github.com/golang-queue/queue"
	"github.com/tus/tusd/pkg/filestore"
	tusd "github.com/tus/tusd/pkg/handler"
)

var tusQueue *queue.Queue
var tusStore *filestore.FileStore
var tusComposer *tusd.StoreComposer

func SetTusQueue(q *queue.Queue) {
	tusQueue = q
}

func GetTusQueue() *queue.Queue {
	return tusQueue
}

func SetTusStore(s *filestore.FileStore) {
	tusStore = s
}

func GetTusStore() *filestore.FileStore {
	return tusStore
}

func SetTusComposer(c *tusd.StoreComposer) {
	tusComposer = c
}

func GetTusComposer() *tusd.StoreComposer {
	return tusComposer
}
