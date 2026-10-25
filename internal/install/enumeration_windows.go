package install

import (
	"io"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Five fixed uninstall values plus one overflow entry; names beyond this fixed
// buffer cannot be one of our ASCII registration names and imply leftovers.
const uninstallValueLimit = 6
const registryNameUnits = 64

var enumPackageValue = windows.NewLazySystemDLL("advapi32.dll").NewProc("RegEnumValueW")

// Read adapters expose requested limits to native fixture tests.
var readPackageDirectory = func(path string, count int) ([]os.DirEntry, error) {
	if count <= 0 || count > 7 {
		return nil, windows.ERROR_INVALID_PARAMETER
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.ReadDir(count)
}
var readPackageValueNames = boundedValueNames
var readPackageSubkeyNames = boundedSubkeyNames

func boundedValueNames(key registry.Key, count int) ([]string, error) {
	if count <= 0 || count > uninstallValueLimit {
		return nil, windows.ERROR_INVALID_PARAMETER
	}
	// x/sys v0.49.0 registry.ReadValueNames preallocates from the entire live
	// ValueCount even for positive n. Call Win32 directly with fixed buffers.
	names := make([]string, 0, count)
	for index := 0; index < count; index++ {
		var name [registryNameUnits]uint16
		length := uint32(len(name))
		status, _, _ := enumPackageValue.Call(uintptr(key), uintptr(index), uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&length)), 0, 0, 0, 0)
		if status == uintptr(windows.ERROR_NO_MORE_ITEMS) {
			return names, io.EOF
		}
		if status != 0 {
			return names, windows.Errno(status)
		}
		if length >= uint32(len(name)) {
			return names, windows.ERROR_INVALID_DATA
		}
		for _, unit := range name[:length] {
			// Embedded terminators or non-ASCII names cannot equal a fixed
			// owned value; do not turn a lossy decoded prefix into authority.
			if unit == 0 || unit > 127 {
				return names, windows.ERROR_INVALID_DATA
			}
		}
		names = append(names, windows.UTF16ToString(name[:length]))
	}
	return names, nil
}

func boundedSubkeyNames(key registry.Key, count int) ([]string, error) {
	if count != 1 {
		return nil, windows.ERROR_INVALID_PARAMETER
	}
	// Only existence matters. A name too long for this one-unit buffer is
	// already an unknown child; return its error conservatively as leftovers.
	var name [1]uint16
	length := uint32(len(name))
	err := windows.RegEnumKeyEx(windows.Handle(key), 0, &name[0], &length, nil, nil, nil, nil)
	if err == windows.ERROR_NO_MORE_ITEMS {
		return nil, io.EOF
	}
	if err != nil {
		return nil, err
	}
	return []string{"present"}, nil
}
