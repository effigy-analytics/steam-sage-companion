package install

import (
	"encoding/binary"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

type packageINI map[string]map[string]string

// Unicode NSIS ReadINIStr expects UTF-16LE with a BOM. Byte limits include it.
// Reject duplicate keys, malformed surrogates, control characters and ambiguous
// quoting instead of accepting a ledger with a different Windows INI meaning.
func decodeINI(raw []byte, limit int) (packageINI, error) {
	if len(raw) < 2 || len(raw) > limit || len(raw)%2 != 0 || raw[0] != 255 || raw[1] != 254 {
		return nil, errors.New("invalid bounded UTF-16LE package record")
	}
	words := make([]uint16, (len(raw)-2)/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(raw[2+i*2:])
	}
	for i := 0; i < len(words); i++ {
		u := words[i]
		if u >= 0xd800 && u <= 0xdbff {
			if i+1 == len(words) || words[i+1] < 0xdc00 || words[i+1] > 0xdfff {
				return nil, errors.New("invalid UTF-16 surrogate")
			}
			i++
		} else if u >= 0xdc00 && u <= 0xdfff {
			return nil, errors.New("invalid UTF-16 surrogate")
		}
	}
	result := packageINI{}
	section := ""
	for _, line := range strings.Split(string(utf16.Decode(words)), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		for _, r := range line {
			if unicode.IsControl(r) {
				return nil, errors.New("invalid package control character")
			}
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			if section == "" || strings.ContainsAny(section, "[]= ") || result[section] != nil || len(result) >= 16 {
				return nil, errors.New("invalid package section")
			}
			result[section] = map[string]string{}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || section == "" || key == "" || strings.TrimSpace(key) != key || strings.TrimSpace(value) != value || strings.ContainsAny(key, "[]\"';") || strings.ContainsAny(value, "\"\x01\x02\x03\x04\x05\x06\x07\x08\x0b\x0c\x0e\x0f") {
			return nil, errors.New("invalid package field")
		}
		if _, exists := result[section][key]; exists {
			return nil, errors.New("duplicate package field")
		}
		result[section][key] = value
	}
	return result, nil
}
func encodeINI(value packageINI, limit int) ([]byte, error) {
	var lines strings.Builder
	sections := make([]string, 0, len(value))
	for section := range value {
		sections = append(sections, section)
	}
	sort.Strings(sections)
	for _, section := range sections {
		if !utf8.ValidString(section) || strings.ContainsAny(section, "\r\n") {
			return nil, errors.New("invalid package section")
		}
		lines.WriteString("[" + section + "]\r\n")
		keys := make([]string, 0, len(value[section]))
		for key := range value[section] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if !utf8.ValidString(key) || strings.ContainsAny(key, "\r\n") {
				return nil, errors.New("invalid package key")
			}
			v := value[section][key]
			if !utf8.ValidString(v) || strings.ContainsAny(v, "\r\n") {
				return nil, errors.New("invalid package string")
			}
			lines.WriteString(key + "=" + v + "\r\n")
		}
	}
	words := utf16.Encode([]rune(lines.String()))
	raw := make([]byte, 2+len(words)*2)
	raw[0] = 255
	raw[1] = 254
	for i, u := range words {
		binary.LittleEndian.PutUint16(raw[2+i*2:], u)
	}
	if _, err := decodeINI(raw, limit); err != nil {
		return nil, err
	}
	return raw, nil
}
