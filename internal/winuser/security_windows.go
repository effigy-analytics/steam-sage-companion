package winuser

import (
	"errors"
	"golang.org/x/sys/windows"
)

func validateDescriptor(sd *windows.SECURITY_DESCRIPTOR, sid string) error {
	return errors.New("private ownership validation is not implemented")
}
