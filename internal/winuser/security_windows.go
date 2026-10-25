package winuser

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const fileAllAccess = 0x001f01ff

func validateDescriptor(sd *windows.SECURITY_DESCRIPTOR, sid string) error {
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || owner.String() != sid {
		return errors.New("settings are not owned by the current user")
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("settings DACL is not protected")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount != 1 {
		return errors.New("settings DACL must allow only the current user")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(acl, 0, &ace); err != nil {
		return err
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&(windows.INHERIT_ONLY_ACE|windows.INHERITED_ACE) != 0 || ace.Mask&fileAllAccess != fileAllAccess {
		return errors.New("settings DACL does not grant private user access")
	}
	if (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String() != sid {
		return errors.New("settings DACL grants another identity access")
	}
	return nil
}

func privateDescriptor(directory bool) (*windows.SECURITY_DESCRIPTOR, error) {
	sid, err := CurrentSID()
	if err != nil {
		return nil, err
	}
	inherit := ""
	if directory {
		inherit = "OICI"
	}
	return windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;" + inherit + ";FA;;;" + sid + ")")
}

// openObject checks the final object by handle, without following its reparse tag.
func openObject(path string, directory bool, access uint32) (windows.Handle, error) {
	if _, err := CanonicalDir(filepath.Dir(path)); err != nil {
		return windows.InvalidHandle, err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	h, err := windows.CreateFile(p, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return windows.InvalidHandle, err
	}
	var info windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(h, &info)
	if err == nil && (info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory) {
		err = errors.New("settings path is a reparse point or wrong type")
	}
	if err != nil {
		windows.CloseHandle(h)
		return windows.InvalidHandle, err
	}
	return h, nil
}

func validatePrivate(path string, directory bool) error {
	h, err := openObject(path, directory, windows.READ_CONTROL)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	sid, err := CurrentSID()
	if err != nil {
		return err
	}
	if err := validateDescriptor(sd, sid); err != nil {
		return err
	}
	if directory {
		acl, _, _ := sd.DACL()
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, 0, &ace); err != nil {
			return err
		}
		if ace.Header.AceFlags&(windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE) != (windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE) {
			return errors.New("private directory does not protect child files")
		}
	}
	return nil
}

func ValidatePrivateDir(path string) error  { return validatePrivate(path, true) }
func ValidatePrivateFile(path string) error { return validatePrivate(path, false) }

// EnsurePrivateDir never repairs existing broad/unowned directories. New directories
// carry the explicit owner and protected DACL from their first filesystem operation.
func EnsurePrivateDir(path string) error {
	canonical, err := CanonicalDir(path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(canonical); err == nil {
		return ValidatePrivateDir(canonical)
	} else if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(canonical)
	if _, err := os.Lstat(parent); os.IsNotExist(err) {
		if err := EnsurePrivateDir(parent); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	sd, err := privateDescriptor(true)
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(canonical)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	if err := windows.CreateDirectory(p, &sa); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	return ValidatePrivateDir(canonical)
}

// ProtectNewFile is only for the empty temporary file just created under an already
// validated private directory. Explicit ownership avoids elevated-token group defaults.
func ProtectNewFile(file *os.File) error {
	if err := ValidatePrivateDir(filepath.Dir(file.Name())); err != nil {
		return err
	}
	h, err := openObject(file.Name(), false, windows.READ_CONTROL|windows.WRITE_DAC|windows.WRITE_OWNER)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	sd, err := privateDescriptor(false)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if err := windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, acl, nil); err != nil {
		return err
	}
	return ValidatePrivateFile(file.Name())
}
