// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// UUID represent a universal identifier with 16 octets.
type UUID [16]byte

// regex for validating that the UUID matches RFC 4122.
// This package generates version 4 UUIDs but
// accepts any UUID version.
// http://www.ietf.org/rfc/rfc4122.txt
var (
	block1 = "[0-9a-f]{8}"
	block2 = "[0-9a-f]{4}"
	block3 = "[0-9a-f]{4}"
	block4 = "[0-9a-f]{4}"
	block5 = "[0-9a-f]{12}"

	UUIDSnippet = block1 + "-" + block2 + "-" + block3 + "-" + block4 + "-" + block5
	validUUID   = regexp.MustCompile("^" + UUIDSnippet + "$")
)

func UUIDFromString(s string) (UUID, error) {
	if !IsValidUUIDString(s) {
		return UUID{}, fmt.Errorf("invalid UUID: %q", s)
	}
	s = strings.Replace(s, "-", "", 4)
	raw, err := hex.DecodeString(s)
	if err != nil {
		return UUID{}, err
	}
	var uuid UUID
	copy(uuid[:], raw)
	return uuid, nil
}

// IsValidUUIDString returns true, if the given string matches a valid UUID (version 4, variant 2).
func IsValidUUIDString(s string) bool {
	return validUUID.MatchString(s)
}

// MustNewUUID returns a new uuid, if an error occurs it panics.
func MustNewUUID() UUID {
	uuid, err := NewUUID()
	if err != nil {
		panic(err)
	}
	return uuid
}

// NewUUID generates a new version 4 UUID relying only on random numbers.
func NewUUID() (UUID, error) {
	uuid := UUID{}
	if _, err := io.ReadFull(rand.Reader, []byte(uuid[0:16])); err != nil {
		return UUID{}, err
	}
	// Set version (4) and variant (2) according to RfC 4122.
	var version byte = 4 << 4
	var variant byte = 8 << 4
	uuid[6] = version | (uuid[6] & 15)
	uuid[8] = variant | (uuid[8] & 15)
	return uuid, nil
}

// Copy returns a copy of the UUID.
func (uuid UUID) Copy() UUID {
	uuidCopy := uuid
	return uuidCopy
}

// Raw returns a copy of the UUID bytes.
func (uuid UUID) Raw() [16]byte {
	return [16]byte(uuid)
}

// String returns a hexadecimal string representation with
// standardized separators.
func (uuid UUID) String() string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
