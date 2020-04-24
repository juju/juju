// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"io"
	"os"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
)

// TODO(ericsnow) Move FingerprintMatcher to charm/resource (or even utils/hash)?

// FingerprintMatcher supports verifying a file's fingerprint.
type FingerprintMatcher struct {
	// Open opens the identified file. It defaults to os.Open.
	Open func(filename string) (io.ReadCloser, error)

	// GenerateFingerprint produces the fingerprint that corresponds
	// to the content of the provided reader. It defaults to
	// charmresource.GenerateFingerprint.
	GenerateFingerprint func(io.Reader) (charmresource.Fingerprint, error)
}

// FingerprintMatches determines whether or not the identified file's
// fingerprint matches the expected fingerprint.
func (fpm FingerprintMatcher) FingerprintMatches(filename string, expected charmresource.Fingerprint) (bool, error) {
	open := fpm.Open
	if open == nil {
		open = func(filename string) (io.ReadCloser, error) { return os.Open(filename) }
	}
	generateFingerprint := fpm.GenerateFingerprint
	if generateFingerprint == nil {
		generateFingerprint = charmresource.GenerateFingerprint
	}

	file, err := open(filename)
	if os.IsNotExist(errors.Cause(err)) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	defer file.Close()

	fp, err := generateFingerprint(file)
	if err != nil {
		return false, errors.Trace(err)
	}
	matches := (fp.String() == expected.String())
	return matches, nil
}
