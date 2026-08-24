package idgen

import (
	"crypto/rand"
	"encoding/binary"
)

func ID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + "-" + hex(raw[:]), nil
}

func RoomCode() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	n := binary.LittleEndian.Uint64(raw[:])
	code := make([]byte, 4)
	for i := range code {
		code[i] = alphabet[n%uint64(len(alphabet))]
		n /= uint64(len(alphabet))
	}
	return "RF-" + string(code), nil
}

func hex(raw []byte) string {
	const chars = "0123456789abcdef"
	out := make([]byte, len(raw)*2)
	for i, value := range raw {
		out[i*2] = chars[value>>4]
		out[i*2+1] = chars[value&0x0f]
	}
	return string(out)
}
