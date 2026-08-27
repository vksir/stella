package collection

import "iter"

type OrderedMap[K comparable, V any] struct {
	index   map[K]int
	entries []entry[K, V]
	deleted int
}

type entry[K comparable, V any] struct {
	key  K
	val  V
	live bool
}

func New[K comparable, V any]() *OrderedMap[K, V] {
	return &OrderedMap[K, V]{index: make(map[K]int)}
}

func (m *OrderedMap[K, V]) Set(key K, val V) {
	if i, ok := m.index[key]; ok {
		m.entries[i].val = val
		return
	}
	m.index[key] = len(m.entries)
	m.entries = append(m.entries, entry[K, V]{key: key, val: val, live: true})
}

func (m *OrderedMap[K, V]) Get(key K) (V, bool) {
	i, ok := m.index[key]
	if !ok {
		var zero V
		return zero, false
	}
	return m.entries[i].val, true
}

func (m *OrderedMap[K, V]) Contains(key K) bool {
	_, ok := m.index[key]
	return ok
}

func (m *OrderedMap[K, V]) Delete(key K) {
	i, ok := m.index[key]
	if !ok {
		return
	}
	delete(m.index, key)
	m.entries[i] = entry[K, V]{}
	m.deleted++
	if m.deleted*2 > len(m.entries) {
		m.compact()
	}
}

func (m *OrderedMap[K, V]) Len() int {
	return len(m.index)
}

func (m *OrderedMap[K, V]) Keys() []K {
	keys := make([]K, 0, len(m.index))
	for _, e := range m.entries {
		if e.live {
			keys = append(keys, e.key)
		}
	}
	return keys
}

func (m *OrderedMap[K, V]) Values() []V {
	vals := make([]V, 0, len(m.index))
	for _, e := range m.entries {
		if e.live {
			vals = append(vals, e.val)
		}
	}
	return vals
}

func (m *OrderedMap[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, e := range m.entries {
			if e.live && !yield(e.key, e.val) {
				return
			}
		}
	}
}

func (m *OrderedMap[K, V]) Clear() {
	clear(m.index)
	m.entries = m.entries[:0]
	m.deleted = 0
}

func (m *OrderedMap[K, V]) compact() {
	n := 0
	for _, e := range m.entries {
		if e.live {
			m.entries[n] = e
			m.index[e.key] = n
			n++
		}
	}
	m.entries = m.entries[:n]
	m.deleted = 0
}
