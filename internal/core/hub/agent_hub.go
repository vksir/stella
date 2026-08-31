package hub

import (
	"log/slog"
	"sync"

	"github.com/vksir/stella/internal/core/agent"
	"github.com/vksir/stella/pkg/collection"
)

const defaultCapacity = 64

type AgentHub struct {
	log      *slog.Logger
	capacity int
	mu       sync.Mutex
	lru      *collection.OrderedMap[string, *agent.Agent]
}

func NewAgentHub(capacity int) *AgentHub {
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	h := &AgentHub{
		log:      slog.Default(),
		lru:      collection.NewOrderedMap[string, *agent.Agent](),
		capacity: capacity,
	}
	return h
}

func (h *AgentHub) GetOrCreate(name string, opts ...agent.Option) (*agent.Agent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entry, ok := h.lru.Get(name)
	if !ok {
		var err error
		entry, err = agent.New(name, opts...)
		if err != nil {
			return nil, err
		}
		h.lru.Set(name, entry)
	}
	h.lru.MoveToEnd(name)
	h.evictLocked()
	return entry, nil
}

func (h *AgentHub) evictLocked() {
	if h.lru.Len() <= h.capacity {
		return
	}
	for _, name := range h.lru.Keys() {
		if h.lru.Len() <= h.capacity {
			break
		}
		h.lru.Delete(name)
		h.log.Info("idle agent evicted", "name", name)
	}
}
