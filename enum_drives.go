//go:build ignore

package main

import (
	"fmt"
	"log"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func main() {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	procGetLogicalDriveStringsW := kernel32.NewProc("GetLogicalDriveStringsW")
	procQueryDosDeviceW := kernel32.NewProc("QueryDosDeviceW")

	// Get all logical drives
	bufSize := 255
	buf := make([]uint16, bufSize)
	ret, _, err := procGetLogicalDriveStringsW.Call(uintptr(bufSize), uintptr(unsafe.Pointer(&buf[0])))
	if ret == 0 {
		log.Fatalf("GetLogicalDriveStringsW failed: %v", err)
	}

	// Parse the null-terminated string array
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

	for _, drive := range drives {
		drivePath, _ := windows.UTF16PtrFromString(drive)

		kernelDriveType := windows.GetDriveType(drivePath)
		driveTypeStr := "Unknown"
		switch kernelDriveType {
		case windows.DRIVE_REMOVABLE:
			driveTypeStr = "Removable"
		case windows.DRIVE_FIXED:
			driveTypeStr = "Fixed"
		case windows.DRIVE_REMOTE:
			driveTypeStr = "Remote"
		case windows.DRIVE_CDROM:
			driveTypeStr = "CD-ROM"
		case windows.DRIVE_RAMDISK:
			driveTypeStr = "RAMDisk"
		case windows.DRIVE_NO_ROOT_DIR:
			driveTypeStr = "NoRoot"
		}

		var volNameBuf [windows.MAX_PATH + 1]uint16
		var fsNameBuf [windows.MAX_PATH + 1]uint16
		var volSerial, maxCompLen, fsFlags uint32

		err = windows.GetVolumeInformation(
			drivePath,
			&volNameBuf[0], uint32(len(volNameBuf)),
			&volSerial,
			&maxCompLen,
			&fsFlags,
			&fsNameBuf[0], uint32(len(fsNameBuf)),
		)

		volName := ""
		fsName := ""
		if err == nil {
			volName = windows.UTF16ToString(volNameBuf[:])
			fsName = windows.UTF16ToString(fsNameBuf[:])
		}

		// Use QueryDosDevice to get the underlying NT device name (often holds the driver indication, e.g. \Device\ImDisk0)
		// QueryDosDevice expects drive letter without backslash (e.g. "C:")
		driveLetter := strings.TrimRight(drive, "\\")
		driveLetterPtr, _ := windows.UTF16PtrFromString(driveLetter)

		var dosDeviceBuf [1024]uint16
		ret, _, _ = procQueryDosDeviceW.Call(
			uintptr(unsafe.Pointer(driveLetterPtr)),
			uintptr(unsafe.Pointer(&dosDeviceBuf[0])),
			uintptr(len(dosDeviceBuf)),
		)

		dosDevice := ""
		if ret != 0 {
			dosDevice = windows.UTF16ToString(dosDeviceBuf[:])
		}

		fmt.Printf("Drive:      %s\n", drive)
		fmt.Printf("Type:       %s (%d)\n", driveTypeStr, kernelDriveType)
		if volName != "" {
			fmt.Printf("Label:      %s\n", volName)
		}
		if fsName != "" {
			fmt.Printf("FileSystem: %s\n", fsName)
		}
		if dosDevice != "" {
			fmt.Printf("Device:     %s\n", dosDevice)
		}
		fmt.Println(strings.Repeat("-", 40))
	}
}
