package main

import (
	"os"
	"path/filepath"
	"testing"

	junction "github.com/nyaosorg/go-windows-junction"
)

func TestNormalizeLinkTargetPathStripsWindowsPrefixes(t *testing.T) {
	baseDir := t.TempDir()
	source := filepath.Join(baseDir, "source")
	expected := filepath.Join(baseDir, "target")

	got, err := normalizeLinkTargetPath(source, `\\?\`+expected)
	if err != nil {
		t.Fatalf("normalizeLinkTargetPath returned error: %v", err)
	}

	if got != expected {
		t.Fatalf("normalizeLinkTargetPath returned %q, want %q", got, expected)
	}
}

func TestLinkPointsToTargetDetectsJunctionDestination(t *testing.T) {
	baseDir := t.TempDir()
	target := filepath.Join(baseDir, "target")
	otherTarget := filepath.Join(baseDir, "other-target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target directory %q: %v", target, err)
	}
	if err := os.MkdirAll(otherTarget, 0o755); err != nil {
		t.Fatalf("create other target directory %q: %v", otherTarget, err)
	}

	source := filepath.Join(baseDir, "junction")
	if err := junction.Create(target, source); err != nil {
		t.Fatalf("create junction %q -> %q: %v", source, target, err)
	}
	t.Cleanup(func() {
		_ = os.Remove(source)
	})

	matches, err := linkPointsToTarget(source, target)
	if err != nil {
		t.Fatalf("linkPointsToTarget returned error for matching target: %v", err)
	}
	if !matches {
		t.Fatalf("linkPointsToTarget(%q, %q) = false, want true", source, target)
	}

	matches, err = linkPointsToTarget(source, otherTarget)
	if err != nil {
		t.Fatalf("linkPointsToTarget returned error for different target: %v", err)
	}
	if matches {
		t.Fatalf("linkPointsToTarget(%q, %q) = true, want false", source, otherTarget)
	}
}

func TestPathEntryExistsDetectsBrokenJunction(t *testing.T) {
	baseDir := t.TempDir()
	target := filepath.Join(baseDir, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target directory %q: %v", target, err)
	}

	source := filepath.Join(baseDir, "goimports")
	if err := junction.Create(target, source); err != nil {
		t.Fatalf("create junction %q -> %q: %v", source, target, err)
	}
	t.Cleanup(func() {
		_ = os.Remove(source)
	})

	if err := os.Remove(target); err != nil {
		t.Fatalf("remove target directory %q: %v", target, err)
	}

	if pathExists(source) {
		t.Fatalf("pathExists(%q) = true, want false for broken junction target", source)
	}
	if !pathEntryExists(source) {
		t.Fatalf("pathEntryExists(%q) = false, want true for broken junction entry", source)
	}
}

func TestPathEntryExistsFalseForMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	if pathEntryExists(missing) {
		t.Fatalf("pathEntryExists(%q) = true, want false", missing)
	}
}

func assertSamePaths(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d paths (%v), want %d (%v)", len(got), got, len(want), want)
	}

	gotSet := make(map[string]struct{}, len(got))
	for _, path := range got {
		gotSet[path] = struct{}{}
	}

	for _, path := range want {
		if _, ok := gotSet[path]; !ok {
			t.Fatalf("missing expected path %q in %v", path, got)
		}
	}
}
