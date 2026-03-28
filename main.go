package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/LogicDaemon/win32linktypes"
	junction "github.com/nyaosorg/go-windows-junction"
	"golang.org/x/sys/windows"
	"gopkg.in/yaml.v3"
)

const (
	example_config = `https://github.com/LogicDaemon/UpdateRAMDiskLinks/blob/master/ramdisk-config.yaml`
)

var (
	runTime        = time.Now()
	dateStr        = runTime.Format("20060102")
	timeStr        = runTime.Format("150405.00")
	ramDrive       string
	timeoutSeconds float64 = -1 // unlimited by default
	configDir      string

	customEnv   = make(map[string]string)
	customEnvMu sync.RWMutex

	aclJobs = make(chan aclJob, 100)
	aclDone = make(chan struct{})

	linesCacheMu sync.RWMutex
	linesCache   = make(map[string]cachedLines)
)

type aclJob struct {
	src, dest string
}

type cachedLines struct {
	lines []string
	err   error
}

func aclWorker() {
	defer close(aclDone)

	ramTemp := filepath.Join(ramDrive, "Temp")
	os.MkdirAll(ramTemp, os.ModePerm)
	tmpACL := filepath.Join(ramTemp, fmt.Sprintf("acl_%d.tmp", time.Now().UnixNano()))

	for job := range aclJobs {
		// Run icacls . /save tmpACL in src directory
		cmdSave := exec.Command("icacls", ".", "/save", tmpACL)
		cmdSave.Dir = job.src
		if err := cmdSave.Run(); err != nil {
			log.Printf("Failed to save ACL for %s: %v", job.src, err)
			continue
		}

		// Run icacls . /restore tmpACL in dest directory
		cmdRestore := exec.Command("icacls", ".", "/restore", tmpACL)
		cmdRestore.Dir = job.dest
		if err := cmdRestore.Run(); err != nil {
			log.Printf("Failed to restore ACL for %s: %v", job.dest, err)
		}

		os.Remove(tmpACL)
	}
}

func findRAMDrive() (string, error) {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	procGetLogicalDriveStringsW := kernel32.NewProc("GetLogicalDriveStringsW")
	procQueryDosDeviceW := kernel32.NewProc("QueryDosDeviceW")

	bufSize := 255
	buf := make([]uint16, bufSize)
	ret, _, err := procGetLogicalDriveStringsW.Call(uintptr(bufSize), uintptr(unsafe.Pointer(&buf[0])))
	if ret == 0 {
		return "", fmt.Errorf("GetLogicalDriveStringsW failed: %v", err)
	}

	var drives []string
	var curr []uint16
	for _, v := range buf {
		if v == 0 {
			if len(curr) > 0 {
				drives = append(drives, windows.UTF16ToString(curr))
				curr = nil
			}
		} else {
			curr = append(curr, v)
		}
	}

	var ramDiskDrives []string
	var ramDiskLabels []string
	var imDiskDrives []string

	imDiskRegex := regexp.MustCompile(`^\\Device\\ImDisk\d+$`)

	for _, drive := range drives {
		drivePath, _ := windows.UTF16PtrFromString(drive)

		kernelDriveType := windows.GetDriveType(drivePath)
		if kernelDriveType == windows.DRIVE_RAMDISK {
			ramDiskDrives = append(ramDiskDrives, drive)
		}

		var volNameBuf [windows.MAX_PATH + 1]uint16
		err = windows.GetVolumeInformation(
			drivePath,
			&volNameBuf[0], uint32(len(volNameBuf)),
			nil, nil, nil, nil, 0,
		)

		if err == nil {
			volName := windows.UTF16ToString(volNameBuf[:])
			if strings.EqualFold(volName, "RamDisk") {
				ramDiskLabels = append(ramDiskLabels, drive)
			}
		}

		driveLetter := strings.TrimRight(drive, "\\")
		driveLetterPtr, _ := windows.UTF16PtrFromString(driveLetter)

		var dosDeviceBuf [1024]uint16
		retDos, _, _ := procQueryDosDeviceW.Call(
			uintptr(unsafe.Pointer(driveLetterPtr)),
			uintptr(unsafe.Pointer(&dosDeviceBuf[0])),
			uintptr(len(dosDeviceBuf)),
		)

		if retDos != 0 {
			dosDevice := windows.UTF16ToString(dosDeviceBuf[:])
			if imDiskRegex.MatchString(dosDevice) {
				imDiskDrives = append(imDiskDrives, drive)
			}
		}
	}

	// Heuristic: if there's exactly one DRIVE_RAMDISK, use it
	if len(ramDiskDrives) == 1 {
		return strings.TrimRight(ramDiskDrives[0], "\\"), nil
	}

	// Otherwise if there's exactly one labeled "RamDisk", use it
	if len(ramDiskLabels) == 1 {
		return strings.TrimRight(ramDiskLabels[0], "\\"), nil
	}

	// Otherwise if there's any ImDisk devices, use the first one
	if len(imDiskDrives) > 0 {
		return strings.TrimRight(imDiskDrives[0], "\\"), nil
	}

	// Otherwise fail
	return "", fmt.Errorf("could not unambiguously find RAM drive (found %d DRIVE_RAMDISK, %d labeled 'RamDisk', %d ImDisk devices)", len(ramDiskDrives), len(ramDiskLabels), len(imDiskDrives))
}

func getEnv(key string) (string, bool) {
	customEnvMu.RLock()
	val, ok := customEnv[key]
	if !ok {
		val, ok = customEnv[strings.ToUpper(key)]
	}
	customEnvMu.RUnlock()
	if ok {
		return val, true
	}

	if val, ok = os.LookupEnv(key); !ok {
		val, ok = os.LookupEnv(strings.ToUpper(key))
	}
	if ok {
		setEnv(key, val)
	}
	return val, ok
}

func setEnv(key, value string) {
	customEnvMu.Lock()
	customEnv[key] = value
	customEnvMu.Unlock()
}

func initEnv() error {
	if ts := os.Getenv("RAMDRIVE_TIMEOUT"); ts != "" {
		if val, err := strconv.ParseFloat(ts, 64); err == nil {
			timeoutSeconds = val
		}
	}

	ramDrive = os.Getenv("RAMDrive")
	if ramDrive == "" {
		foundDrive, err := findRAMDrive()
		if err != nil {
			return err
		}
		ramDrive = foundDrive
		setEnv("RAMDrive", ramDrive)
	}
	// Also ensure APPDATA, LOCALAPPDATA, USERPROFILE are populated from standard Go os.UserHomeDir() if they're empty
	if os.Getenv("USERPROFILE") == "" {
		h, _ := os.UserHomeDir()
		setEnv("USERPROFILE", h)
	}
	userProfile, _ := getEnv("USERPROFILE")
	if os.Getenv("APPDATA") == "" {
		setEnv("APPDATA", filepath.Join(userProfile, "AppData", "Roaming"))
	}
	if os.Getenv("LOCALAPPDATA") == "" {
		setEnv("LOCALAPPDATA", filepath.Join(userProfile, "AppData", "Local"))
	}
	return nil
}

func waitForRAMDrive() error {
	if pathExists(ramDrive + "\\") {
		return nil
	}

	log.Printf("Warning: RAMDrive (%s) not ready. Waiting...\n", ramDrive)

	start := time.Now()
	for {
		if pathExists(ramDrive + "\\") {
			log.Printf("RAMDrive (%s) is now ready.\n", ramDrive)
			return nil
		}
		if timeoutSeconds > 0 && time.Since(start).Seconds() > timeoutSeconds {
			return fmt.Errorf("timeout waiting for RAMDrive (%s)", ramDrive)
		}
		time.Sleep(1 * time.Second)
	}
}

func expandEnv(s string) (string, error) {
	var buf strings.Builder
	buf.Grow(len(s))

	for i := 0; i < len(s); {
		if s[i] == '%' {
			if i+1 < len(s) && s[i+1] == '%' {
				buf.WriteByte('%')
				i += 2
				continue
			}

			end := strings.IndexByte(s[i+1:], '%')
			if end == -1 {
				buf.WriteByte('%')
				i++
				continue
			}

			varName := s[i+1 : i+1+end]
			if val, ok := getEnv(varName); ok {
				buf.WriteString(val)
			} else {
				return "", fmt.Errorf("undefined environment variable: %s", varName)
			}

			i += end + 2
		} else {
			buf.WriteByte(s[i])
			i++
		}
	}
	return buf.String(), nil
}

func mkDirs(valNode *yaml.Node, basePath string) {
	if valNode.Kind == yaml.ScalarNode {
		if valNode.Value == "" {
			return
		}
		dirPath, err := expandEnv(valNode.Value)
		if err != nil {
			log.Printf("Skipping mkdir for '%s': %v\n", valNode.Value, err)
			return
		}
		var fullPath string
		if filepath.IsAbs(dirPath) {
			fullPath = dirPath
		} else if basePath != "" {
			fullPath = filepath.Join(basePath, dirPath)
		} else {
			fullPath = filepath.Join(ramDrive, dirPath)
			log.Printf("Warning: Root mkdir path '%s' is relative. Resolved against RAM Drive to '%s'\n", dirPath, fullPath)
		}
		createDirectory(fullPath, "")
	} else if valNode.Kind == yaml.SequenceNode {
		for _, n := range valNode.Content {
			mkDirs(n, basePath)
		}
	} else if valNode.Kind == yaml.MappingNode {
		for j := 0; j < len(valNode.Content); j += 2 {
			keyNode := valNode.Content[j]
			childNode := valNode.Content[j+1]

			dirPath, err := expandEnv(keyNode.Value)
			if err != nil {
				log.Printf("Skipping mkdir for '%s': %v\n", keyNode.Value, err)
				continue
			}

			var fullPath string
			if filepath.IsAbs(dirPath) {
				fullPath = dirPath
			} else if basePath != "" {
				fullPath = filepath.Join(basePath, dirPath)
			} else {
				fullPath = filepath.Join(ramDrive, dirPath)
				log.Printf("Warning: Root mkdir path '%s' is relative. Resolved against RAM Drive to '%s'\n", dirPath, fullPath)
			}

			createDirectory(fullPath, "")

			if childNode.Kind != 0 && (childNode.Kind != yaml.ScalarNode || childNode.Value != "") {
				mkDirs(childNode, fullPath)
			}
		}
	}
}

func processNode(basePath string, node *yaml.Node) {
	if node.Kind != yaml.MappingNode {
		return
	}

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		key := keyNode.Value

		if strings.HasPrefix(key, ":") {
			switch key {
			case ":defs":
				// e.g. ":defs" just skips processing this as a physical path
				// but we should still parse anchors if any.
				continue
			case ":mkdir":
				mkDirs(valNode, basePath)
			}
		}

		if strings.HasPrefix(key, "<") {
			fileName := key[1:]
			var err error
			fileName, err = expandEnv(fileName)
			if err != nil {
				log.Printf("Skipping file include '%s': %v\n", key, err)
				continue
			}
			if !filepath.IsAbs(fileName) && configDir != "" {
				fileName = filepath.Join(configDir, fileName)
			}
			lines, err := readLinesMemoized(fileName)
			if err == nil {
				for _, line := range lines {
					processPath(basePath, line, valNode)
				}
			}
			continue
		}

		processPath(basePath, key, valNode)
	}
}

func createDirectory(dirPath string, basePath string) {
	if !filepath.IsAbs(dirPath) && basePath != "" {
		dirPath = filepath.Join(basePath, dirPath)
	}
	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
		log.Printf("Failed to create directory '%s': %v\n", dirPath, err)
	}
}

func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func readLinesMemoized(path string) ([]string, error) {
	cacheKey := filepath.Clean(path)

	linesCacheMu.RLock()
	entry, ok := linesCache[cacheKey]
	linesCacheMu.RUnlock()
	if ok {
		return entry.lines, entry.err
	}

	lines, err := readLines(path)

	linesCacheMu.Lock()
	linesCache[cacheKey] = cachedLines{lines: lines, err: err}
	linesCacheMu.Unlock()

	return lines, err
}

func processPath(basePath, key string, valNode *yaml.Node) {
	checkExists := false
	if strings.HasPrefix(key, "?") {
		checkExists = true
		key = key[1:]
	}

	if key == "." {
		// Just a check for existence of the current basePath
		if checkExists && !pathExists(basePath) {
			return
		}
		// If valNode has children, process them against basePath
		if valNode.Kind == yaml.MappingNode {
			processNode(basePath, valNode)
		}
		return
	}

	var err error
	key, err = expandEnv(key)
	if err != nil {
		log.Printf("Skipping path '%s': %v\n", key, err)
		return
	}

	var fullPath string
	if filepath.IsAbs(key) {
		fullPath = key
	} else if basePath != "" {
		fullPath = filepath.Join(basePath, key)
	} else {
		fullPath = filepath.Join(configDir, key)
		log.Printf("Warning: Root path '%s' is relative. Resolved against configuration directory to '%s'\n", key, fullPath)
	}

	// Globbing
	if strings.ContainsAny(fullPath, "*?") {
		matches, err := filepath.Glob(fullPath)
		if err == nil && len(matches) > 0 {
			for _, match := range matches {
				processResolvedPath(match, checkExists, valNode)
			}
			return
		}
	}

	processResolvedPath(fullPath, checkExists, valNode)
}

func processResolvedPath(fullPath string, checkExists bool, valNode *yaml.Node) {
	if checkExists && !pathExists(fullPath) {
		return
	}

	if valNode.Kind == yaml.MappingNode {
		isTargetOverride := false
		for i := 0; i < len(valNode.Content); i += 2 {
			if valNode.Content[i].Value == ">" {
				isTargetOverride = true
				targetNode := valNode.Content[i+1]
				handleOverride(fullPath, targetNode)
				break
			}
		}

		if !isTargetOverride {
			if len(valNode.Content) == 0 {
				linkToRAMDisk(fullPath)
			} else {
				if !path.IsAbs(fullPath) {
					log.Printf("ERROR: processResolvedPath got a relative input directory '%s'", fullPath)
					return
				}
				ramTarget := getRAMTarget(fullPath)
				if err := mkdirWithACL(fullPath, ramTarget); err != nil {
					log.Printf("Failed to create intermediate target directory %s: %v\n", ramTarget, err)
				}
				processNode(fullPath, valNode)
			}
		}
	} else if valNode.Kind == yaml.ScalarNode || valNode.Kind == yaml.SequenceNode {
		// Just in case it's a bare target override or something
		if valNode.Value == "" {
			linkToRAMDisk(fullPath)
		}
	} else if valNode.Kind == yaml.AliasNode {
		// Inherit mapping
		if valNode.Alias.Kind == yaml.MappingNode {
			processNode(fullPath, valNode.Alias)
		} else if len(valNode.Alias.Content) == 0 {
			linkToRAMDisk(fullPath)
		}
	} else if valNode.Value == "" && valNode.Kind == 0 {
		linkToRAMDisk(fullPath)
	} else if len(valNode.Content) == 0 {
		linkToRAMDisk(fullPath)
	}
}

func tryGetTargetFromGlob(t string) (string, bool) {
	matches, err := filepath.Glob(t)
	if err == nil && len(matches) > 0 {
		for _, match := range matches {
			if pathExists(match) {
				return match, true
			}
		}
	}
	return "", false
}

func handleOverride(fullPath string, targetNode *yaml.Node) {
	var target string

	if targetNode.Kind == yaml.ScalarNode {
		t, err := expandEnv(targetNode.Value)
		if err != nil {
			log.Printf("Skipping override '%s' for '%s': %v\n", targetNode.Value, fullPath, err)
			return
		}
		if !filepath.IsAbs(t) {
			t = filepath.Join(getRAMTarget(filepath.Dir(fullPath)), t)
		}
		if strings.ContainsAny(t, "*?") {
			if match, found := tryGetTargetFromGlob(t); found {
				target = match
			}
		} else {
			target = t
		}
	} else if targetNode.Kind == yaml.SequenceNode {
		for _, n := range targetNode.Content {
			t, err := expandEnv(n.Value)
			if err != nil {
				log.Printf("Skipping override option '%s' for '%s': %v\n", n.Value, fullPath, err)
				continue
			}
			if !filepath.IsAbs(t) {
				t = filepath.Join(getRAMTarget(filepath.Dir(fullPath)), t)
			}
			if strings.ContainsAny(t, "*?") {
				if match, found := tryGetTargetFromGlob(t); found {
					target = match
					break
				}
			} else if pathExists(t) {
				target = t
				break
			}
		}
	}

	if target != "" {
		makeLink(fullPath, target)
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func getRAMTarget(source string) string {
	if !filepath.IsAbs(source) {
		log.Printf("ERROR: getRAMTarget got a relative input path '%s'", source)
		return ""
	}

	drivePrefix := filepath.VolumeName(source)
	relPath := source[len(drivePrefix):]
	if len(relPath) > 0 && os.IsPathSeparator(relPath[0]) {
		relPath = relPath[1:]
	}

	ramDriveClean := ramDrive
	if !strings.HasSuffix(ramDriveClean, "\\") && !strings.HasSuffix(ramDriveClean, "/") {
		ramDriveClean += string(filepath.Separator)
	}

	return filepath.Join(ramDriveClean, relPath)
}

func linkToRAMDisk(source string) {
	makeLink(source, getRAMTarget(source))
}

func mkdirWithACL(srcDir, dstDir string) error {
	if pathExists(dstDir) {
		return nil
	}

	var dirsToCreate []struct{ src, dst string }

	curSrc := srcDir
	curDst := dstDir

	// Walk up until we find an existing destination directory or hit the root
	for !pathExists(curDst) {
		dirsToCreate = append(dirsToCreate, struct{ src, dst string }{curSrc, curDst})

		nextSrc := filepath.Dir(curSrc)
		nextDst := filepath.Dir(curDst)

		// Stop if we can't go any higher
		if nextDst == curDst {
			break
		}
		curSrc = nextSrc
		curDst = nextDst
	}

	// Create the entire destination tree in one operation
	if err := os.MkdirAll(dstDir, os.ModePerm); err != nil {
		return err
	}

	// Queue ACL copies from parents to leaves
	for i := len(dirsToCreate) - 1; i >= 0; i-- {
		pair := dirsToCreate[i]

		// Ensure the destination is actually a directory before queueing ACL copy
		if dstInfo, err := os.Stat(pair.dst); err != nil || !dstInfo.IsDir() {
			continue
		}

		aclSrc := pair.src

		if srcInfo, err := os.Stat(aclSrc); err == nil && srcInfo.IsDir() {
			aclJobs <- aclJob{src: aclSrc, dest: pair.dst}
		}
	}

	return nil
}

func makeLink(source, target string) {
	log.Printf("Linking %s -> %s\n", source, target)

	var isFile bool
	sourceExists := true
	info, err := os.Lstat(source)

	if err == nil {
		linkType, tErr := win32linktypes.GetType(source)
		if tErr == nil && linkType != win32linktypes.TypeNormal {
			if linkType == win32linktypes.TypeFileSymlink {
				isFile = true
			} else {
				isFile = false
			}
		} else {
			isFile = !info.IsDir()
		}
	} else if os.IsNotExist(err) {
		sourceExists = false
		isFile = false
	} else {
		log.Printf("Error accessing source %s: %v\n", source, err)
		return
	}

	// 1. create target on RAM
	if isFile {
		if err := mkdirWithACL(filepath.Dir(source), filepath.Dir(target)); err != nil {
			log.Printf("Failed to create target directories for %s: %v\n", target, err)
		}
		// Touch target file if it doesn't exist
		if _, err := os.Stat(target); os.IsNotExist(err) {
			f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
			if err == nil {
				f.Close()
			}
		}
	} else {
		if err := mkdirWithACL(source, target); err != nil {
			log.Printf("Failed to create target directories for %s: %v\n", target, err)
		}
	}

	// 2. remove/rename source
	if sourceExists {
		linkType, tErr := win32linktypes.GetType(source)
		if tErr == nil && linkType != win32linktypes.TypeNormal {
			os.Remove(source)
		} else {
			removed := false
			if isFile {
				if info.Size() == 0 {
					if err := os.Remove(source); err != nil {
						log.Printf("Failed to remove empty file %s: %v\n", source, err)
						return
					}
					removed = true
				}
			} else {
				if err := os.Remove(source); err == nil {
					removed = true
				}
			}

			if !removed {
				renameTo := source + ".LINKED_" + dateStr + "_" + timeStr
				if err := os.Rename(source, renameTo); err != nil {
					log.Printf("Failed to rename %s to %s: %v\n", source, renameTo, err)
					return
				}
			}
		}
	} else {
		// If source does not exist and wasn't skipped by '?', ensure parent exists so we can create symlink
		os.MkdirAll(filepath.Dir(source), os.ModePerm)
	}

	// 3. create junction/symlink at the source path pointing to the RAM target
	if isFile {
		if err := os.Symlink(target, source); err != nil {
			log.Printf("Failed to create file symlink %s -> %s: %v\n", source, target, err)
		}
	} else {
		if err := junction.Create(target, source); err != nil {
			// Fallback to directory symlink if junction fails
			if errSym := os.Symlink(target, source); errSym != nil {
				log.Printf("Failed to create junction/symlink %s -> %s: %v\n", source, target, err)
			}
		}
	}
}

func main() {
	if err := initEnv(); err != nil {
		log.Fatalf("Error: %v\n", err)
	}

	if err := waitForRAMDrive(); err != nil {
		log.Fatalf("Error: %v\n", err)
	}

	go aclWorker()
	defer func() {
		close(aclJobs)
		<-aclDone
	}()

	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <config.yaml>\nConfig file example:\n%s", os.Args[0], example_config)
	}
	configPath := os.Args[1]
	if absPath, err := filepath.Abs(configPath); err == nil {
		configDir = filepath.Dir(absPath)
	} else {
		configDir = filepath.Dir(configPath)
		if configDir == "." {
			configDir, _ = os.Getwd()
		}
	}

	b, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Error reading config: %v\n", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		log.Fatalf("Error unmarshaling YAML: %v\n", err)
	}

	if len(root.Content) == 0 {
		return
	}
	processNode("", root.Content[0])
}
