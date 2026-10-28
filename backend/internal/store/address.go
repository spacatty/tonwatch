package store

import (
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
)

// FriendlyAddress converts a raw TON address ("0:abcd…") to the bounceable base64url form
// used by Getgems/Tonviewer links. Returns "" if raw can't be parsed.
func FriendlyAddress(raw string) string {
	wcStr, hexPart, ok := strings.Cut(raw, ":")
	if !ok {
		return ""
	}
	wc, err := strconv.Atoi(wcStr)
	hash, err2 := hex.DecodeString(hexPart)
	if err != nil || err2 != nil || len(hash) != 32 {
		return ""
	}
	b := make([]byte, 0, 36)
	b = append(b, 0x11, byte(int8(wc)))
	b = append(b, hash...)
	crc := crc16(b)
	b = append(b, byte(crc>>8), byte(crc))
	return base64.URLEncoding.EncodeToString(b)
}

func crc16(data []byte) uint16 {
	var crc uint16
	for _, by := range data {
		crc ^= uint16(by) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
