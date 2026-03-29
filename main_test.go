package main

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/LogicDaemon/win32linktypes"
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

func parseMappingNode(t *testing.T, yamlText string) *yaml.Node {
	t.Helper()

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlText), &root); err != nil {
		t.Fatalf("unmarshal YAML mapping: %v", err)
	}
	if len(root.Content) == 0 {
		t.Fatal("parsed YAML mapping is empty")
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		t.Fatalf("parsed YAML node kind = %v, want mapping", doc.Kind)
	}

	return doc
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

func resetSetupLogStateForTest(t *testing.T, testConfigDir string) {
	t.Helper()

	prevConfigDir := configDir
	prevWriter := log.Writer()
	prevLogValue, prevLogWasSet := os.LookupEnv("LOG")

	customEnvMu.Lock()
	prevCustomEnv := customEnv
	customEnv = make(map[string]string)
	customEnvMu.Unlock()

	closeLogFile()
	log.SetOutput(os.Stderr)
	configDir = testConfigDir

	t.Cleanup(func() {
		closeLogFile()
		log.SetOutput(prevWriter)
		configDir = prevConfigDir
		if prevLogWasSet {
			_ = os.Setenv("LOG", prevLogValue)
		} else {
			_ = os.Unsetenv("LOG")
		}

		customEnvMu.Lock()
		customEnv = prevCustomEnv
		customEnvMu.Unlock()
	})
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

func TestResolveLogPathExpandsEnvVarsInExplicitPath(t *testing.T) {
	baseDir := t.TempDir()
	resetSetupLogStateForTest(t, baseDir)

	logsRoot := filepath.Join(baseDir, "logs-root")
	t.Setenv("TEST_LOG_ROOT", logsRoot)

	expectedLogPath := filepath.Join(logsRoot, "app.log")
	got, err := resolveLogPath(`%TEST_LOG_ROOT%\app.log`)
	if err != nil {
		t.Fatalf("resolveLogPath returned error: %v", err)
	}

	if got != expectedLogPath {
		t.Fatalf("resolveLogPath returned %q, want %q", got, expectedLogPath)
	}
}

func TestResolveLogPathUsesLOGEnvironmentValueAsIs(t *testing.T) {
	baseDir := t.TempDir()
	resetSetupLogStateForTest(t, baseDir)

	expectedLogPath := filepath.Join(baseDir, "from-env.log")
	if err := os.Setenv("LOG", expectedLogPath); err != nil {
		t.Fatalf("set LOG environment: %v", err)
	}
	if got := os.Getenv("LOG"); got != expectedLogPath {
		t.Fatalf("LOG environment before setupLog = %q, want %q", got, expectedLogPath)
	}

	got, err := resolveLogPath("")
	if err != nil {
		t.Fatalf("resolveLogPath returned error: %v", err)
	}

	if got != expectedLogPath {
		t.Fatalf("resolveLogPath returned %q, want %q", got, expectedLogPath)
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

func TestResolveConfigPathsExcludesClaimedSameLevelMatches(t *testing.T) {
	baseDir := t.TempDir()
	explicitPath := filepath.Join(baseDir, "asdasf", "vcxvxc")
	anotherPath := filepath.Join(baseDir, "other", "child")

	for _, dir := range []string{explicitPath, anotherPath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create directory %q: %v", dir, err)
		}
	}

	excluded := make(claimedPathSet)
	excluded.add(explicitPath)

	got, err := resolveConfigPaths(baseDir, `*\*`, excluded)
	if err != nil {
		t.Fatalf("resolveConfigPaths returned error: %v", err)
	}

	assertSamePaths(t, got, []string{anotherPath})
}

func TestResolveConfigPathsKeepsDifferentLevelMatches(t *testing.T) {
	baseDir := t.TempDir()
	parentPath := filepath.Join(baseDir, "asdasf")
	childPath := filepath.Join(parentPath, "vcxvxc")
	otherPath := filepath.Join(baseDir, "other")

	for _, dir := range []string{childPath, otherPath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create directory %q: %v", dir, err)
		}
	}

	excluded := make(claimedPathSet)
	excluded.add(childPath)

	got, err := resolveConfigPaths(baseDir, `*`, excluded)
	if err != nil {
		t.Fatalf("resolveConfigPaths returned error: %v", err)
	}

	assertSamePaths(t, got, []string{parentPath, otherPath})
}

func TestResolveConfigPathsOptionalMissingReturnsEmpty(t *testing.T) {
	baseDir := t.TempDir()

	got, err := resolveConfigPaths(baseDir, `?missing`, nil)
	if err != nil {
		t.Fatalf("resolveConfigPaths returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("resolveConfigPaths returned %v, want empty result for missing optional path", got)
	}
}

func TestResolveConfigPathsClaimsExactPath(t *testing.T) {
	baseDir := t.TempDir()
	explicitPath := filepath.Join(baseDir, "asdasf", "vcxvxc")
	if err := os.MkdirAll(explicitPath, 0o755); err != nil {
		t.Fatalf("create directory %q: %v", explicitPath, err)
	}

	excluded := make(claimedPathSet)
	got, err := resolveConfigPaths(baseDir, `asdasf\vcxvxc`, excluded)
	if err != nil {
		t.Fatalf("resolveConfigPaths returned error: %v", err)
	}

	assertSamePaths(t, got, []string{explicitPath})
	if !excluded.has(explicitPath) {
		t.Fatalf("resolveConfigPaths did not claim explicit path %q", explicitPath)
	}
}

func TestResolveConfigPathsDoesNotClaimGlobMatches(t *testing.T) {
	baseDir := t.TempDir()
	matchPath := filepath.Join(baseDir, "asdasf", "vcxvxc")
	if err := os.MkdirAll(matchPath, 0o755); err != nil {
		t.Fatalf("create directory %q: %v", matchPath, err)
	}

	excluded := make(claimedPathSet)
	got, err := resolveConfigPaths(baseDir, `*\*`, excluded)
	if err != nil {
		t.Fatalf("resolveConfigPaths returned error: %v", err)
	}

	assertSamePaths(t, got, []string{matchPath})
	if excluded.has(matchPath) {
		t.Fatalf("resolveConfigPaths unexpectedly claimed glob match %q", matchPath)
	}
}

func TestProcessNodeGlobBeforeExplicitSkipExcludesReservedPath(t *testing.T) {
	defer drainACLJobsForTest()
	aclJobsWg = sync.WaitGroup{}

	baseDir := t.TempDir()
	previousRAMDrive := ramDrive
	ramDrive = filepath.Join(baseDir, "ram")
	t.Cleanup(func() {
		ramDrive = previousRAMDrive
		drainACLJobsForTest()
	})

	localState := filepath.Join(baseDir, "source", "LocalState")
	cacheDir := filepath.Join(localState, "Cache")
	ebWebViewDir := filepath.Join(localState, "EBWebView")

	for _, dir := range []string{cacheDir, ebWebViewDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create source directory %q: %v", dir, err)
		}
	}

	node := parseMappingNode(t, `
"*":
"EBWebView": ":skip"
`)

	processNode(localState, node)

	cacheType, err := win32linktypes.GetType(cacheDir)
	if err != nil {
		t.Fatalf("GetType(%q): %v", cacheDir, err)
	}
	if cacheType == win32linktypes.TypeNormal {
		t.Fatalf("Cache directory %q was not converted into a link", cacheDir)
	}

	ebWebViewType, err := win32linktypes.GetType(ebWebViewDir)
	if err != nil {
		t.Fatalf("GetType(%q): %v", ebWebViewDir, err)
	}
	if ebWebViewType != win32linktypes.TypeNormal {
		t.Fatalf("EBWebView directory %q should remain unchanged when reserved with :skip", ebWebViewDir)
	}

	ramCacheDir := getRAMTarget(cacheDir)
	if !pathExists(ramCacheDir) {
		t.Fatalf("expected RAM target %q to be created for Cache", ramCacheDir)
	}

	ramEBWebViewDir := getRAMTarget(ebWebViewDir)
	if pathExists(ramEBWebViewDir) {
		t.Fatalf("unexpected RAM target %q created for reserved :skip directory", ramEBWebViewDir)
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
