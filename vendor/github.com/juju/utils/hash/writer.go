// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package hash

import (
	"encoding/base64"
	"hash"
	"io"
)

// TODO(ericsnow) Remove HashingWriter and NewHashingWriter().

// HashingWriter wraps an io.Writer, providing the checksum of all data
// written to it.  A HashingWriter may be used in place of the writer it
// wraps.
//
// Note: HashingWriter is deprecated. Please do not use it. We will
// remove it ASAP.
type HashingWriter struct {
	hash    hash.Hash
	wrapped io.Writer
}

// NewHashingWriter returns a new HashingWriter that wraps the provided
// writer and the hasher.
//
// Example:
//   hw := NewHashingWriter(w, sha1.New())
//   io.Copy(hw, reader)
//   hash := hw.Base64Sum()
//
// Note: NewHashingWriter is deprecated. Please do not use it. We will
// remove it ASAP.
func NewHashingWriter(writer io.Writer, hasher hash.Hash) *HashingWriter {
	return &HashingWriter{
		hash:    hasher,
		wrapped: io.MultiWriter(writer, hasher),
	}
}

// Base64Sum returns the base64 encoded hash.
func (hw HashingWriter) Base64Sum() string {
	sumBytes := hw.hash.Sum(nil)
	return base64.StdEncoding.EncodeToString(sumBytes)
}

// Write writes to both the wrapped file and the hash.
func (hw *HashingWriter) Write(data []byte) (int, error) {
	// No trace because some callers, like ioutil.ReadAll(), won't work.
	return hw.wrapped.Write(data)
}
