// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource

import (
	stdhash "hash"
	"io"

	"github.com/juju/errors"
	"github.com/juju/utils/hash"
)

var newHash, validateSum = hash.SHA384()

// Fingerprint represents the unique fingerprint value of a resource's data.
type Fingerprint struct {
	hash.Fingerprint
}

// NewFingerprint returns wraps the provided raw fingerprint bytes.
// This function roundtrips with Fingerprint.Bytes().
func NewFingerprint(raw []byte) (Fingerprint, error) {
	fp, err := hash.NewFingerprint(raw, validateSum)
	if err != nil {
		return Fingerprint{}, errors.Trace(err)
	}
	return Fingerprint{fp}, nil
}

// ParseFingerprint returns wraps the provided raw fingerprint string.
// This function roundtrips with Fingerprint.String().
func ParseFingerprint(raw string) (Fingerprint, error) {
	fp, err := hash.ParseHexFingerprint(raw, validateSum)
	if err != nil {
		return Fingerprint{}, errors.Trace(err)
	}
	return Fingerprint{fp}, nil
}

// GenerateFingerprint returns the fingerprint for the provided data.
func GenerateFingerprint(reader io.Reader) (Fingerprint, error) {
	fp, err := hash.GenerateFingerprint(reader, newHash)
	if err != nil {
		return Fingerprint{}, errors.Trace(err)
	}
	return Fingerprint{fp}, nil
}

// Fingerprint is a hash that may be used to generate fingerprints.
type FingerprintHash struct {
	stdhash.Hash
}

// NewFingerprintHash returns a hash that may be used to create fingerprints.
func NewFingerprintHash() *FingerprintHash {
	return &FingerprintHash{
		Hash: newHash(),
	}
}

// Fingerprint returns the current fingerprint of the hash.
func (fph FingerprintHash) Fingerprint() Fingerprint {
	fp := hash.NewValidFingerprint(fph)
	return Fingerprint{fp}
}
