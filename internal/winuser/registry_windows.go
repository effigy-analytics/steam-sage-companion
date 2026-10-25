package winuser

import (
	"encoding/binary"
	"errors"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

// ReadRegistryString establishes exact REG_SZ identity without GetStringValue's
// unbounded allocation, NUL truncation or replacement of malformed UTF-16. A
// changed/ill-formed value remains present and must never be overwritten/deleted.
func ReadRegistryString(key registry.Key, name string) (string, bool, error) {
	raw := make([]byte, 16*1024)
	n, kind, err := key.GetValue(name, raw)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", true, err
	}
	if kind != registry.SZ || n < 2 || n > len(raw) || n%2 != 0 {
		return "", true, errors.New("registry value is not a bounded terminated REG_SZ")
	}
	raw = raw[:n]
	if raw[n-2] != 0 || raw[n-1] != 0 {
		return "", true, errors.New("registry string has no terminator")
	}
	words := make([]uint16, n/2-1)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(raw[2*i:])
		if words[i] == 0 {
			return "", true, errors.New("registry string has an embedded NUL")
		}
	}
	for i := 0; i < len(words); i++ {
		u := words[i]
		if u >= 0xd800 && u <= 0xdbff {
			if i+1 == len(words) || words[i+1] < 0xdc00 || words[i+1] > 0xdfff {
				return "", true, errors.New("registry string has malformed UTF-16")
			}
			i++
		} else if u >= 0xdc00 && u <= 0xdfff {
			return "", true, errors.New("registry string has malformed UTF-16")
		}
	}
	return string(utf16.Decode(words)), true, nil
}
