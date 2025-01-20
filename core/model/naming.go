// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
)

const (
	// uuidSuffixDigits defines how many of the uuid digits to use.
	// Since the suffix function asserts that the modelUUID is valid, we know
	// it follows the UUID string format that ends with eight hex digits.
	uuidSuffixDigits = 6

	// minMaxLength is the minimum allowed maxLength value.
	minMaxLength = 16
)

func suffix(modelUUID string) (string, error) {
	if !names.IsValidModel(modelUUID) {
		return "", errors.NotValidf("model UUID %q", modelUUID)
	}
	// The suffix is the last six hex digits of the model uuid.
	return modelUUID[len(modelUUID)-uuidSuffixDigits:], nil
}

// DisambiguateResourceName creates a unique resource name from the supplied name by
// appending a suffix derived from the model UUID, up to a length of maxLength.
// To achieve maxLength, the name is right trimmed.
func DisambiguateResourceName(modelUUID string, name string, maxLength uint) (string, error) {
	if maxLength < minMaxLength {
		return "", fmt.Errorf("maxLength (%d) must be greater than %d", maxLength, minMaxLength)
	}
	if overflow := len(name) + 1 + uuidSuffixDigits - int(maxLength); overflow > 0 {
		name = name[0 : len(name)-overflow]
	}
	suffix, err := suffix(modelUUID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", name, suffix), nil
}
