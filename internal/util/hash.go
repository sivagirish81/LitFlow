package util

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
)

func SHA256HexFromReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func SHA256Hex(b []byte) string {
	x := sha256.Sum256(b)
	return hex.EncodeToString(x[:])
}
