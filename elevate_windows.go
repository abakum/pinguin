//go:build windows

package main

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32           = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
)

func isAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)
	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

func runMeElevated() {
	fmt.Println("Requesting administrator privileges...")
	exe, _ := os.Executable()
	cwd, _ := os.Getwd()
	verbPtr, _ := windows.UTF16PtrFromString("runas")
	exePtr, _ := windows.UTF16PtrFromString(exe)
	cwdPtr, _ := windows.UTF16PtrFromString(cwd)

	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(exePtr)),
		0,
		uintptr(unsafe.Pointer(cwdPtr)),
		uintptr(1),
	)

	if ret <= 32 {
		fmt.Printf("Error: Failed to elevate privileges (Code: %d)\n", ret)
		os.Exit(1)
	}
}

// elevateIfNeeded restarts the process elevated unless it already runs as admin
func elevateIfNeeded() {
	if !isAdmin() {
		runMeElevated()
		os.Exit(0)
	}
}
