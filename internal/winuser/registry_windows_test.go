package winuser

import (
	"golang.org/x/sys/windows/registry"
	"testing"
)

// Decoder input is the actual returned byte sequence. This covers unterminated
// data even on Windows versions that normalize a RegSetValueExW writer fixture.
func TestStrictRegistryDecoderRequiresCompleteCanonicalUTF16(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  []byte
	}{
		{"unterminated", []byte{'o', 0}},
		{"odd bytes", []byte{'o', 0, 0}},
		{"embedded NUL", []byte{'o', 0, 0, 0, 'x', 0, 0, 0}},
		{"unpaired high surrogate", []byte{0, 0xd8, 0, 0}},
		{"unpaired low surrogate", []byte{0, 0xdc, 0, 0}},
		{"oversize", make([]byte, 16*1024+2)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeRegistryString(test.raw, registry.SZ); err == nil {
				t.Fatal("accepted malformed raw registry data")
			}
		})
	}
	for _, test := range []struct {
		raw  []byte
		want string
	}{
		{[]byte{'o', 0, 0, 0}, "o"},
		{[]byte{0xea, 0x96, 0, 0}, "雪"},
		{[]byte{0, 0}, ""},
	} {
		value, err := decodeRegistryString(test.raw, registry.SZ)
		if err != nil || value != test.want {
			t.Fatalf("decode=%q err=%v want=%q", value, err, test.want)
		}
	}
	if _, err := decodeRegistryString([]byte{'o', 0, 0, 0}, registry.EXPAND_SZ); err == nil {
		t.Fatal("accepted a different registry type")
	}
}
