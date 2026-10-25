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
	if n < 0 || n > len(raw) {
		return "", true, errors.New("invalid registry byte count")
	}
	value, err := decodeRegistryString(raw[:n], kind)
	return value, true, err
}

func decodeRegistryString(raw []byte, kind uint32) (string, error) {
	if kind != registry.SZ || len(raw) < 2 || len(raw) > 16*1024 || len(raw)%2 != 0 {
		return "", errors.New("registry value is not a bounded terminated REG_SZ")
	}
	n := len(raw)
	if raw[n-2] != 0 || raw[n-1] != 0 {
		return "", errors.New("registry string has no terminator")
	}
	words := make([]uint16, n/2-1)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(raw[2*i:])
		if words[i] == 0 {
			return "", errors.New("registry string has an embedded NUL")
		}
	}
	for i := 0; i < len(words); i++ {
		u := words[i]
		if u >= 0xd800 && u <= 0xdbff {
			if i+1 == len(words) || words[i+1] < 0xdc00 || words[i+1] > 0xdfff {
				return "", errors.New("registry string has malformed UTF-16")
			}
			i++
		} else if u >= 0xdc00 && u <= 0xdfff {
			return "", errors.New("registry string has malformed UTF-16")
		}
	}
	return string(utf16.Decode(words)), nil
}
