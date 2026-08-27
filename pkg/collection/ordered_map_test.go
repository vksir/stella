package collection

import (
	"reflect"
	"testing"
)

func TestOrderedMap(t *testing.T) {
	m := New[string, int]()

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
	m := New[int, int]()
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

func TestOrderedMapAll(t *testing.T) {
	m := New[string, int]()
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
