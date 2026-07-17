package db

import (
	"encoding/base64"
	"net/url"
	"unicode"
	"unicode/utf8"
)

func isPrintableUTF8(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		if !unicode.IsPrint(r) && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func decodeFilterValue(s string) string {
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		decoded = s
	}
	if decoded == "finished" || decoded == "in-progress" || decoded == "not-started" || decoded == "no-series" {
		return decoded
	}
	data, err := base64.StdEncoding.DecodeString(decoded)
	if err != nil {
		return decoded
	}
	if !isPrintableUTF8(data) {
		return decoded
	}
	return string(data)
}
