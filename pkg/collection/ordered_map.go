package collection

import "iter"

// OrderedMap 按插入顺序保存键值，零值可用。
// nil 接收者视为空映射，写操作无效。
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

func NewOrderedMap[K comparable, V any]() *OrderedMap[K, V] {
	return &OrderedMap[K, V]{index: make(map[K]int)}
}

func (m *OrderedMap[K, V]) Set(key K, val V) {
	if m == nil {
		return
	}
	if m.index == nil {
		m.index = make(map[K]int)
	}
	if i, ok := m.index[key]; ok {
		m.entries[i].val = val
		return
	}
	m.index[key] = len(m.entries)
	m.entries = append(m.entries, entry[K, V]{key: key, val: val, live: true})
}

func (m *OrderedMap[K, V]) Get(key K) (V, bool) {
	if m == nil {
		var zero V
		return zero, false
	}
	i, ok := m.index[key]
	if !ok {
		var zero V
		return zero, false
	}
	return m.entries[i].val, true
}

func (m *OrderedMap[K, V]) Contains(key K) bool {
	if m == nil {
		return false
	}
	_, ok := m.index[key]
	return ok
}

func (m *OrderedMap[K, V]) Delete(key K) {
	if m == nil {
		return
	}
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
	if m == nil {
		return 0
	}
	return len(m.index)
}

func (m *OrderedMap[K, V]) Keys() []K {
	if m == nil {
		return nil
	}
	keys := make([]K, 0, len(m.index))
	for _, e := range m.entries {
		if e.live {
			keys = append(keys, e.key)
		}
	}
	return keys
}

func (m *OrderedMap[K, V]) Values() []V {
	if m == nil {
		return nil
	}
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
		if m == nil {
			return
		}
		for _, e := range m.entries {
			if e.live && !yield(e.key, e.val) {
				return
			}
		}
	}
}

// Clone 返回浅拷贝，nil 接收者返回 nil。
func (m *OrderedMap[K, V]) Clone() *OrderedMap[K, V] {
	if m == nil {
		return nil
	}
	c := &OrderedMap[K, V]{
		index:   make(map[K]int, len(m.index)),
		entries: append([]entry[K, V](nil), m.entries...),
		deleted: m.deleted,
	}
	for k, v := range m.index {
		c.index[k] = v
	}
	return c
}

// MoveToEnd TODO: 性能
func (m *OrderedMap[K, V]) MoveToEnd(key K) {
	if m == nil {
		return
	}
	i, ok := m.index[key]
	if !ok {
		return
	}
	e := m.entries[i]
	copy(m.entries[i:], m.entries[i+1:])
	m.entries[len(m.entries)-1] = e
	for j := i; j < len(m.entries); j++ {
		if m.entries[j].live {
			m.index[m.entries[j].key] = j
		}
	}
}

func (m *OrderedMap[K, V]) Clear() {
	if m == nil {
		return
	}
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
