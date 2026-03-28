package main

import (
	"os"
	"path/filepath"
	"testing"

	junction "github.com/nyaosorg/go-windows-junction"
	"gopkg.in/yaml.v3"
)

func parseRootDirectiveNode(t *testing.T, yamlText, key string) *yaml.Node {
	t.Helper()

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlText), &root); err != nil {
		t.Fatalf("unmarshal YAML: %v", err)
	}
	if len(root.Content) == 0 {
		t.Fatal("parsed YAML document is empty")
	}

	doc := root.Content[0]
	for i := 0; i < len(doc.Content); i += 2 {
		if doc.Content[i].Value == key {
			return doc.Content[i+1]
		}
	}

	t.Fatalf("directive %q not found in YAML", key)
	return nil
}

func drainACLJobsForTest() {
	for {
		select {
		case <-aclJobs:
			aclJobsWg.Done()
		default:
			return
		}
	}
}

func TestParseDirectiveBool(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    bool
		wantErr bool
	}{
		{
			name: "true literal enables directive",
			yaml: `":uselinkstarget": true`,
			want: true,
		},
		{
			name: "empty value enables directive",
			yaml: `":uselinkstarget":`,
			want: true,
		},
		{
			name: "off disables directive",
			yaml: `":uselinkstarget": off`,
			want: false,
		},
		{
			name:    "mapping value is rejected",
			yaml:    `":uselinkstarget": { "nested": true }`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := parseRootDirectiveNode(t, tt.yaml, ":uselinkstarget")
			got, err := parseDirectiveBool(":uselinkstarget", node)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseDirectiveBool returned nil error, want failure")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDirectiveBool returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseDirectiveBool returned %v, want %v", got, tt.want)
			}
		})
	}
}

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

func TestMakeLinkUsesExistingTargetWhenEnabled(t *testing.T) {
	defer drainACLJobsForTest()

	previousSetting := useExistingLinksTarget
	useExistingLinksTarget = true
	t.Cleanup(func() {
		useExistingLinksTarget = previousSetting
		drainACLJobsForTest()
	})

	baseDir := t.TempDir()
	source := filepath.Join(baseDir, "cache-link")
	existingTarget := filepath.Join(baseDir, "special-disk", "cache")
	configuredTarget := filepath.Join(baseDir, "ram-disk", "cache")

	if err := os.MkdirAll(existingTarget, 0o755); err != nil {
		t.Fatalf("create existing target %q: %v", existingTarget, err)
	}
	if err := junction.Create(existingTarget, source); err != nil {
		t.Fatalf("create junction %q -> %q: %v", source, existingTarget, err)
	}
	t.Cleanup(func() {
		_ = os.Remove(source)
	})

	if err := os.RemoveAll(existingTarget); err != nil {
		t.Fatalf("remove existing target %q: %v", existingTarget, err)
	}

	makeLink(source, configuredTarget)

	matches, err := linkPointsToTarget(source, existingTarget)
	if err != nil {
		t.Fatalf("linkPointsToTarget returned error for preserved target: %v", err)
	}
	if !matches {
		t.Fatalf("existing link %q was not preserved to %q", source, existingTarget)
	}

	matches, err = linkPointsToTarget(source, configuredTarget)
	if err != nil {
		t.Fatalf("linkPointsToTarget returned error for configured target: %v", err)
	}
	if matches {
		t.Fatalf("existing link %q was unexpectedly repointed to %q", source, configuredTarget)
	}

	info, err := os.Stat(existingTarget)
	if err != nil {
		t.Fatalf("stat recreated existing target %q: %v", existingTarget, err)
	}
	if !info.IsDir() {
		t.Fatalf("recreated existing target %q is not a directory", existingTarget)
	}

	if pathEntryExists(configuredTarget) {
		t.Fatalf("configured RAM target %q should not be created when preserving existing link target", configuredTarget)
	}
}

func TestMkDirsNestedRelativePathsUseMirroredParentTarget(t *testing.T) {
	baseDir := t.TempDir()
	previousRAMDrive := ramDrive
	ramDrive = filepath.Join(baseDir, "ram")
	t.Cleanup(func() {
		ramDrive = previousRAMDrive
	})

	sourceBase := filepath.Join(baseDir, "source", "Users", "Example", "AppData", "Local", "Steam")
	node := parseRootDirectiveNode(t, `
":mkdir":
  "appcache\\httpcache":
  "depotcache":
  "logs":
`, ":mkdir")

	mkDirs(node, sourceBase)

	expectedBase := getRAMTarget(sourceBase)
	for _, relativePath := range []string{
		filepath.Join("appcache", "httpcache"),
		"depotcache",
		"logs",
	} {
		expectedPath := filepath.Join(expectedBase, relativePath)
		info, err := os.Stat(expectedPath)
		if err != nil {
			t.Fatalf("stat mirrored mkdir path %q: %v", expectedPath, err)
		}
		if !info.IsDir() {
			t.Fatalf("mirrored mkdir path %q is not a directory", expectedPath)
		}

		unexpectedPath := filepath.Join(sourceBase, relativePath)
		if pathExists(unexpectedPath) {
			t.Fatalf("relative :mkdir path %q was created under the live source path", unexpectedPath)
		}
	}
}

func TestMkDirsRootRelativePathsUseRAMDriveRoot(t *testing.T) {
	baseDir := t.TempDir()
	previousRAMDrive := ramDrive
	ramDrive = filepath.Join(baseDir, "ram")
	t.Cleanup(func() {
		ramDrive = previousRAMDrive
	})

	node := parseRootDirectiveNode(t, `
":mkdir":
  "Temp":
  "cache\\steam":
`, ":mkdir")

	mkDirs(node, "")

	for _, expectedPath := range []string{
		filepath.Join(ramDriveRoot(), "Temp"),
		filepath.Join(ramDriveRoot(), "cache", "steam"),
	} {
		info, err := os.Stat(expectedPath)
		if err != nil {
			t.Fatalf("stat RAM root mkdir path %q: %v", expectedPath, err)
		}
		if !info.IsDir() {
			t.Fatalf("RAM root mkdir path %q is not a directory", expectedPath)
		}
	}
}

func TestMkDirsAbsolutePathsStayAbsolute(t *testing.T) {
	baseDir := t.TempDir()
	previousRAMDrive := ramDrive
	ramDrive = filepath.Join(baseDir, "ram")
	t.Cleanup(func() {
		ramDrive = previousRAMDrive
	})

	sourceBase := filepath.Join(baseDir, "source", "Users", "Example", "AppData", "Local", "Steam")
	absTarget := filepath.Join(baseDir, "absolute-target", "Steam", "logs")
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: absTarget}

	mkDirs(node, sourceBase)

	info, err := os.Stat(absTarget)
	if err != nil {
		t.Fatalf("stat absolute mkdir path %q: %v", absTarget, err)
	}
	if !info.IsDir() {
		t.Fatalf("absolute mkdir path %q is not a directory", absTarget)
	}

	mirroredPath := filepath.Join(getRAMTarget(sourceBase), "logs")
	if pathExists(mirroredPath) {
		t.Fatalf("absolute :mkdir path unexpectedly created mirrored path %q", mirroredPath)
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
