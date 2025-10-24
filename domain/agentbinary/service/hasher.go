// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	cryptosha256 "crypto/sha256"
	cryptosha512 "crypto/sha512"
	"encoding/hex"
	"io"
)

func computeSHA256andSHA384(r io.Reader) (io.Reader, func() (string, string)) {
	hasher256 := cryptosha256.New()
	hasher384 := cryptosha512.New384()
	return io.TeeReader(r, io.MultiWriter(hasher256, hasher384)), func() (string, string) {
		return hex.EncodeToString(hasher256.Sum(nil)),
			hex.EncodeToString(hasher384.Sum(nil))
	}
}
