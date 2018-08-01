// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package hash

import (
	"encoding/base64"
	"encoding/hex"
	"hash"
	"io"

	"github.com/juju/errors"
)

// Fingerprint represents the checksum for some data.
type Fingerprint struct {
	sum []byte
}

// NewFingerprint returns wraps the provided raw hash sum. This function
// roundtrips with Fingerprint.Bytes().
func NewFingerprint(sum []byte, validate func([]byte) error) (Fingerprint, error) {
	if validate == nil {
		return Fingerprint{}, errors.New("missing validate func")
	}

	if err := validate(sum); err != nil {
		return Fingerprint{}, errors.Trace(err)
	}
	return newFingerprint(sum), nil
}

// NewValidFingerprint returns a Fingerprint corresponding
// to the current of the provided hash.
func NewValidFingerprint(hash hash.Hash) Fingerprint {
	sum := hash.Sum(nil)
	return newFingerprint(sum)
}

func newFingerprint(sum []byte) Fingerprint {
	return Fingerprint{
		sum: append([]byte{}, sum...), // Use an isolated copy.
	}
}

// GenerateFingerprint returns the fingerprint for the provided data.
func GenerateFingerprint(reader io.Reader, newHash func() hash.Hash) (Fingerprint, error) {
	var fp Fingerprint

	if reader == nil {
		return fp, errors.New("missing reader")
	}
	if newHash == nil {
		return fp, errors.New("missing new hash func")
	}

	hash := newHash()
	if _, err := io.Copy(hash, reader); err != nil {
		return fp, errors.Trace(err)
	}
	fp.sum = hash.Sum(nil)
	return fp, nil
}

// ParseHexFingerprint returns wraps the provided raw fingerprint string.
// This function roundtrips with Fingerprint.Hex().
func ParseHexFingerprint(hexSum string, validate func([]byte) error) (Fingerprint, error) {
	if validate == nil {
		return Fingerprint{}, errors.New("missing validate func")
	}

	sum, err := hex.DecodeString(hexSum)
	if err != nil {
		return Fingerprint{}, errors.Trace(err)
	}
	fp, err := NewFingerprint(sum, validate)
	if err != nil {
		return Fingerprint{}, errors.Trace(err)
	}
	return fp, nil
}

// ParseBase64Fingerprint returns wraps the provided raw fingerprint string.
// This function roundtrips with Fingerprint.Base64().
func ParseBase64Fingerprint(b64Sum string, validate func([]byte) error) (Fingerprint, error) {
	if validate == nil {
		return Fingerprint{}, errors.New("missing validate func")
	}

	sum, err := base64.StdEncoding.DecodeString(b64Sum)
	if err != nil {
		return Fingerprint{}, errors.Trace(err)
	}
	fp, err := NewFingerprint(sum, validate)
	if err != nil {
		return Fingerprint{}, errors.Trace(err)
	}
	return fp, nil
}

// String implements fmt.Stringer.
func (fp Fingerprint) String() string {
	return fp.Hex()
}

// Hex returns the hex string representation of the fingerprint.
func (fp Fingerprint) Hex() string {
	return hex.EncodeToString(fp.sum)
}

// Base64 returns the base64 encoded fingerprint.
func (fp Fingerprint) Base64() string {
	return base64.StdEncoding.EncodeToString(fp.sum)
}

// Bytes returns the raw (sum) bytes of the fingerprint.
func (fp Fingerprint) Bytes() []byte {
	return append([]byte{}, fp.sum...)
}

// IsZero returns whether or not the fingerprint is the zero value.
func (fp Fingerprint) IsZero() bool {
	return len(fp.sum) == 0
}

// Validate returns an error if the fingerprint is invalid.
func (fp Fingerprint) Validate() error {
	if fp.IsZero() {
		return errors.NotValidf("zero-value fingerprint")
	}
	return nil
}
