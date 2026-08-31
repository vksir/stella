package collection

import (
	"reflect"
	"testing"
)

func TestOrderedMapNilReceiver(t *testing.T) {
	var m *OrderedMap[string, int]
	m.Set("key", 1)
	m.Delete("key")
	m.MoveToEnd("key")
	m.Clear()

	if value, ok := m.Get("key"); value != 0 || ok {
		t.Fatalf("Get() = (%d, %v), want (0, false)", value, ok)
	}
	if m.Contains("key") || m.Len() != 0 {
		t.Fatalf("nil map is not empty: contains=%v len=%d", m.Contains("key"), m.Len())
	}
	if keys := m.Keys(); keys != nil {
		t.Fatalf("Keys() = %v, want nil", keys)
	}
	if values := m.Values(); values != nil {
		t.Fatalf("Values() = %v, want nil", values)
	}
	if cloned := m.Clone(); cloned != nil {
		t.Fatalf("Clone() = %v, want nil", cloned)
	}
	for range m.All() {
		t.Fatal("All() yielded an entry for a nil map")
	}
}

func TestOrderedMapZeroValue(t *testing.T) {
	var m OrderedMap[string, int]
	m.Set("key", 1)
	if value, ok := m.Get("key"); value != 1 || !ok {
		t.Fatalf("Get() = (%d, %v), want (1, true)", value, ok)
	}
	if m.Len() != 1 || !reflect.DeepEqual(m.Keys(), []string{"key"}) {
		t.Fatalf("zero-value map insertion failed: len=%d keys=%v", m.Len(), m.Keys())
	}
}

func TestOrderedMap(t *testing.T) {
	m := NewOrderedMap[string, int]()

	m.Set("b", 2)
	m.Set("a", 1)
	m.Set("c", 3)
	if !reflect.DeepEqual(m.Keys(), []string{"b", "a", "c"}) {
		t.Fatalf("insert order broken: %v", m.Keys())
	}

	m.Set("a", 10)
	if !reflect.DeepEqual(m.Keys(), []string{"b", "a", "c"}) {
		t.Fatalf("update moved key: %v", m.Keys())
	}
	if v, _ := m.Get("a"); v != 10 {
		t.Fatalf("update lost: %v", v)
	}

	m.Delete("b")
	if !reflect.DeepEqual(m.Keys(), []string{"a", "c"}) {
		t.Fatalf("delete broke order: %v", m.Keys())
	}
	if m.Contains("b") || m.Len() != 2 {
		t.Fatalf("delete state wrong: has=%v len=%d", m.Contains("b"), m.Len())
	}
}

func TestOrderedMapCompact(t *testing.T) {
	m := NewOrderedMap[int, int]()
	for i := range 10 {
		m.Set(i, i)
	}
	for i := range 7 {
		m.Delete(i)
	}
	want := []int{7, 8, 9}
	if !reflect.DeepEqual(m.Keys(), want) {
		t.Fatalf("compact broke order: %v", m.Keys())
	}
	m.Set(11, 11)
	want = append(want, 11)
	if !reflect.DeepEqual(m.Keys(), want) {
		t.Fatalf("set after compact broken: %v", m.Keys())
	}
}

func TestMoveToEndSkipsDeletedEntries(t *testing.T) {
	m := NewOrderedMap[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Delete("b")
	m.MoveToEnd("a")
	if m.Len() != 1 || m.Contains("") {
		t.Fatalf("moving across a deleted entry corrupted the index: len=%d keys=%v", m.Len(), m.Keys())
	}
	m.Set("", 3)
	m.Set("c", 4)
	m.Delete("c")
	m.MoveToEnd("a")
	if value, ok := m.Get(""); !ok || value != 3 {
		t.Fatalf("zero key was overwritten by a deleted entry: value=%d ok=%v", value, ok)
	}
}

func TestOrderedMapAll(t *testing.T) {
	m := NewOrderedMap[string, int]()
	m.Set("x", 1)
	m.Set("y", 2)
	var got []string
	for k, v := range m.All() {
		got = append(got, k)
		_ = v
	}
	if !reflect.DeepEqual(got, []string{"x", "y"}) {
		t.Fatalf("All order broken: %v", got)
	}
}
