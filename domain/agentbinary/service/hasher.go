// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	cryptosha256 "crypto/sha256"
	cryptosha512 "crypto/sha512"
	"encoding/hex"
	"io"
)

// ComputedHashes holds the computed SHA256 and SHA384 hashes of a binary.
type ComputedHashes struct {
	SHA256 string
	SHA384 string
}

func computeSHA256andSHA384(r io.Reader) (io.Reader, func() ComputedHashes) {
	hasher256 := cryptosha256.New()
	hasher384 := cryptosha512.New384()
	return io.TeeReader(r, io.MultiWriter(hasher256, hasher384)), func() ComputedHashes {
		return ComputedHashes{
			SHA256: hex.EncodeToString(hasher256.Sum(nil)),
			SHA384: hex.EncodeToString(hasher384.Sum(nil)),
		}
	}
}
