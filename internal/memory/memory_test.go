package memory

import (
	"os"
	"strings"
	"testing"
)

func TestStoreAppendAndAll(t *testing.T) {
	tmp := t.TempDir() + "/mem.jsonl"
	m := New(tmp)
	if err := m.Append([]string{"a", "b"}, "hello world", "test"); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := m.Append([]string{"c"}, "second entry", "test"); err != nil {
		t.Fatalf("Append 2: %v", err)
	}
	all, err := m.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 entries, got %d", len(all))
	}
	if all[0].Content != "hello world" {
		t.Errorf("entry 0 content: got %q", all[0].Content)
	}
}

func TestStoreSearch(t *testing.T) {
	tmp := t.TempDir() + "/mem.jsonl"
	m := New(tmp)
	_ = m.Append([]string{"fruit"}, "apple is red", "test")
	_ = m.Append([]string{"fruit"}, "banana is yellow", "test")
	_ = m.Append([]string{"vehicle"}, "car has wheels", "test")

	hits, err := m.Search("fruit", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Errorf("expected 2 fruit hits, got %d", len(hits))
	}

	hits, _ = m.Search("yellow", 10)
	if len(hits) != 1 {
		t.Errorf("expected 1 yellow hit, got %d", len(hits))
	}

	hits, _ = m.Search("zzz", 10)
	if len(hits) != 0 {
		t.Errorf("expected 0 hits for zzz, got %d", len(hits))
	}
}

func TestStoreCorruptJSONLIsSkipped(t *testing.T) {
	tmp := t.TempDir() + "/mem.jsonl"
	_ = os.WriteFile(tmp, []byte(strings.Join([]string{
		"garbage line",
		`{"id":"1","tags":["a"],"content":"good","createdAt":"2026-01-01T00:00:00Z"}`,
		"more garbage",
		`{"id":"2","tags":["b"],"content":"also good","createdAt":"2026-01-02T00:00:00Z"}`,
	}, "\n")), 0o644)

	m := New(tmp)
	all, err := m.All()
	if err != nil {
		t.Errorf("All() should not fail on corrupt lines: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 valid entries, got %d", len(all))
	}
}

func TestStoreDeleteByID(t *testing.T) {
	tmp := t.TempDir() + "/mem.jsonl"
	m := New(tmp)
	_ = m.Append([]string{"a"}, "first", "test")
	_ = m.Append([]string{"b"}, "second", "test")
	all, _ := m.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries before delete, got %d", len(all))
	}
	if err := m.DeleteByID(all[0].ID); err != nil {
		t.Fatalf("DeleteByID: %v", err)
	}
	all, _ = m.All()
	if len(all) != 1 {
		t.Errorf("expected 1 entry after delete, got %d", len(all))
	}
	if all[0].Content != "second" {
		t.Errorf("wrong entry left after delete: got %q", all[0].Content)
	}
}

func TestStoreClear(t *testing.T) {
	tmp := t.TempDir() + "/mem.jsonl"
	m := New(tmp)
	_ = m.Append([]string{"a"}, "first", "test")
	_ = m.Append([]string{"b"}, "second", "test")
	if m.Count() != 2 {
		t.Errorf("expected 2 before clear, got %d", m.Count())
	}
	if err := m.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if m.Count() != 0 {
		t.Errorf("expected 0 after clear, got %d", m.Count())
	}
}
