package settings

import (
	"testing"
	"time"
)

func TestCacheServesRepeatReadsWithoutTouchingTheStore(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetModuleEnabled(-1, "aiAnswer", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}

	c := NewCache(s, time.Minute)
	if !c.ModuleEnabled(-1, "aiAnswer") {
		t.Fatal("ModuleEnabled = false; want true")
	}

	// Правка мимо кэша: пока TTL не истёк, кэш обязан отдавать старое.
	if err := s.SetModuleEnabled(-1, "aiAnswer", false); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}
	if !c.ModuleEnabled(-1, "aiAnswer") {
		t.Error("ModuleEnabled = false before the TTL expired; want the cached true")
	}
}

func TestCacheRefreshesAfterTTL(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetModuleEnabled(-1, "aiAnswer", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}

	now := time.Unix(1000, 0)
	c := NewCache(s, 5*time.Second)
	c.now = func() time.Time { return now }

	if !c.ModuleEnabled(-1, "aiAnswer") {
		t.Fatal("ModuleEnabled = false; want true")
	}

	if err := s.SetModuleEnabled(-1, "aiAnswer", false); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}
	now = now.Add(6 * time.Second)

	if c.ModuleEnabled(-1, "aiAnswer") {
		t.Error("ModuleEnabled = true after the TTL expired; want the fresh false")
	}
}

func TestCacheKeysByChatAndModule(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetModuleEnabled(-1, "aiAnswer", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}

	c := NewCache(s, time.Minute)

	if !c.ModuleEnabled(-1, "aiAnswer") {
		t.Error("ModuleEnabled(-1, aiAnswer) = false; want true")
	}
	if c.ModuleEnabled(-1, "skazka") {
		t.Error("ModuleEnabled(-1, skazka) = true; want false")
	}
	if c.ModuleEnabled(-2, "aiAnswer") {
		t.Error("ModuleEnabled(-2, aiAnswer) = true; want false")
	}
}
