package requestid

import (
	"unicode"
	"unicode/utf8"
)

const MaxLength = 128

func Valid(value string) bool {
	if value == "" || len(value) > MaxLength || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
