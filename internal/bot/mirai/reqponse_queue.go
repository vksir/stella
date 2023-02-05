package mirai

import (
	"math/rand"
	"strconv"
)

type responseQueues map[string]chan *EventReceive

func (r responseQueues) register() (string, chan *EventReceive) {
	q := make(chan *EventReceive)
	var syncId string
	for {
		syncId = strconv.Itoa(rand.Int())
		if _, ok := r[syncId]; !ok {
			break
		}
	}
	r[syncId] = q
	return syncId, q
}

func (r responseQueues) unRegister(syncId string) {
	delete(r, syncId)
}

func (r responseQueues) putResponse(e *EventReceive) {
	if queue, ok := r[e.SyncId]; ok {
		queue <- e
	} else {
		log.Info("putResponse failed")
	}
}
