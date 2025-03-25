// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"strings"

	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

const (
	// DefaultSuffixDigits defines how many of the uuid digits to use.
	// Since the suffix function asserts that the modelUUID is valid, we know
	// it follows the UUID string format that ends with eight hex digits.
	DefaultSuffixDigits = uint(6)

	// maxSuffixLength is the maximum number of UUID digits to use.
	maxSuffixLength = 32

	// minMaxNameLength is the minimum allowed maxNameLength value.
	minMaxNameLength = 16

	// minResourceNameComponentLength is the minimum length, including separator,
	// of the resource name component of the disambiguated name.
	minResourceNameComponentLength = 5
)

func suffix(modelUUID string, suffixLength uint) (string, error) {
	if !names.IsValidModel(modelUUID) {
		return "", errors.Errorf("model UUID %q %w", modelUUID, coreerrors.NotValid)
	}
	// The suffix is the last six hex digits of the model uuid.
	modelUUIDDigitsOnly := strings.ReplaceAll(modelUUID, "-", "")
	return modelUUIDDigitsOnly[len(modelUUIDDigitsOnly)-int(suffixLength):], nil
}

// DisambiguateResourceName creates a unique resource name from the supplied name by
// appending a suffix derived from the model UUID.
// The maximum length of the entire resulting resource name is maxLength.
// To achieve maxLength, the name is right trimmed.
// The default suffix length [DefaultSuffixDigits] is used.
func DisambiguateResourceName(modelUUID string, name string, maxLength uint) (string, error) {
	return DisambiguateResourceNameWithSuffixLength(modelUUID, name, maxLength, DefaultSuffixDigits)
}

// DisambiguateResourceNameWithSuffixLength creates a unique resource name from the supplied name by
// appending a suffix derived from the model UUID, using the specified suffix length.
// The maximum length of the entire resulting resource name is maxLength.
// To achieve maxLength, the name is right trimmed.
// The default suffix length [DefaultSuffixDigits] is used.
func DisambiguateResourceNameWithSuffixLength(modelUUID string, name string, maxNameLength, suffixLength uint) (string, error) {
	if maxNameLength < minMaxNameLength {
		return "", errors.Errorf("maxNameLength (%d) must be greater than %d", maxNameLength, minMaxNameLength)
	}
	var maxAllowedSuffixLength uint = maxSuffixLength
	if maxAllowedSuffixLength > maxNameLength-minResourceNameComponentLength {
		maxAllowedSuffixLength = maxNameLength - minResourceNameComponentLength
	}
	if suffixLength < DefaultSuffixDigits || suffixLength > maxAllowedSuffixLength {
		return "", errors.Errorf("suffixLength (%d) must be between %d and %d", suffixLength, DefaultSuffixDigits, maxAllowedSuffixLength)
	}
	if overflow := len(name) + 1 + int(suffixLength) - int(maxNameLength); overflow > 0 {
		name = name[0 : len(name)-overflow]
	}
	suffix, err := suffix(modelUUID, suffixLength)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", name, suffix), nil
}
