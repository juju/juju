// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// The hash package provides utilities that support use of the stdlib
// hash.Hash. Most notably is the Fingerprint type that wraps the
// checksum of a hash.
//
// Conversion between checksums and strings are facailitated through
// Fingerprint.
//
// Here are some hash-related recipes that bring it all together:
//
// * Extract the SHA384 hash while writing to elsewhere, then get the
//   raw checksum:
//
//     newHash, _ := hash.SHA384()
//     h := newHash()
//     hashingWriter := io.MultiWriter(writer, h)
//     if err := writeAll(hashingWriter); err != nil { ... }
//     fp := hash.NewValidFingerprint(h)
//     checksum := fp.Bytes()
//
// * Extract the SHA384 hash while reading from elsewhere, then get the
//   hex-encoded checksum to send over the wire:
//
//     newHash, _ := hash.SHA384()
//     h := newHash()
//     hashingReader := io.TeeReader(reader, h)
//     if err := processStream(hashingReader); err != nil { ... }
//     fp := hash.NewValidFingerprint(h)
//     hexSum := fp.Hex()
//     req.Header.Set("Content-Sha384", hexSum)
//
// * Turn a checksum sent over the wire back into a fingerprint:
//
//     _, validate := hash.SHA384()
//     hexSum := req.Header.Get("Content-Sha384")
//     var fp hash.Fingerprint
//     if len(hexSum) != 0 {
//         fp, err = hash.ParseHexFingerprint(hexSum, validate)
//         ...
//     }
//     if fp.IsZero() {
//         ...
//     }
package hash

import (
	"crypto/sha512"
	"hash"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("utils.hash")

// SHA384 returns the newHash and validate functions for use
// with SHA384 hashes. SHA384 is used in several key places in Juju.
func SHA384() (newHash func() hash.Hash, validate func([]byte) error) {
	const digestLenBytes = 384 / 8
	validate = newSizeChecker(digestLenBytes)
	return sha512.New384, validate
}

func newSizeChecker(size int) func([]byte) error {
	return func(sum []byte) error {
		if len(sum) < size {
			return errors.NewNotValid(nil, "invalid fingerprint (too small)")
		}
		if len(sum) > size {
			return errors.NewNotValid(nil, "invalid fingerprint (too big)")
		}
		return nil
	}
}
