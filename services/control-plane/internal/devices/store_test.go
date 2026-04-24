package devices

import (
	"errors"
	"reflect"
	"testing"
)

func TestMemStore_CRUD(t *testing.T) {
	s := NewMemStore()

	if err := s.Create(Device{Serial: "DEV-1", Tags: []string{"yocto-wic-ab", "x86"}, PublicKey: "PEM"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(Device{Serial: "DEV-1"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("want ErrAlreadyExists, got %v", err)
	}

	d, err := s.Get("DEV-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Tags, []string{"x86", "yocto-wic-ab"}) {
		t.Fatalf("tags not sorted/unique: %v", d.Tags)
	}

	if _, err := s.Get("MISSING"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestMemStore_TagAndFilterAND(t *testing.T) {
	s := NewMemStore()
	_ = s.Create(Device{Serial: "A", Tags: []string{"yocto", "x86"}})
	_ = s.Create(Device{Serial: "B", Tags: []string{"yocto", "arm"}})
	_ = s.Create(Device{Serial: "C", Tags: []string{"yocto", "x86", "ros2"}})

	got, err := s.List([]string{"yocto", "x86"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Serial != "A" || got[1].Serial != "C" {
		t.Fatalf("AND filter wrong: %+v", got)
	}
}

func TestMemStore_UpdateTagsAndRetire(t *testing.T) {
	s := NewMemStore()
	_ = s.Create(Device{Serial: "A", Tags: []string{"old"}})

	d, err := s.UpdateTags("A", []string{"new", "extra"}, []string{"old"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Tags, []string{"extra", "new"}) {
		t.Fatalf("tag update wrong: %v", d.Tags)
	}

	if err := s.Retire("A", "EOL"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.List(nil)
	if len(got) != 0 {
		t.Fatalf("retired devices must not appear in list, got %d", len(got))
	}
}
