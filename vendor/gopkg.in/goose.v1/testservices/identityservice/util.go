package identityservice

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

type UserInfo struct {
	Id       string
	TenantId string
	Token    string
	secret   string
}

var randReader = rand.Reader

// Generate a bit of random hex data for
func randomHexToken() string {
	raw_bytes := make([]byte, 16)
	n, err := io.ReadFull(randReader, raw_bytes)
	if err != nil {
		panic(fmt.Sprintf(
			"failed to read 16 random bytes (read %d bytes): %s",
			n, err.Error()))
	}
	hex_bytes := make([]byte, 32)
	// hex.Encode can't fail, no error checking needed.
	hex.Encode(hex_bytes, raw_bytes)
	return string(hex_bytes)
}
