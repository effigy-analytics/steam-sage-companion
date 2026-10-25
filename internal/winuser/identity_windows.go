package winuser

import "golang.org/x/sys/windows"

// CurrentSID identifies the process user, including when its default owner is a group.
func CurrentSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}
