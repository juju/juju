// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"testing"

	"github.com/juju/tc"
)

// TestInvalidModelNames tests a set of known invalid model names to make sure
// they don't pass validation.
func TestInvalidModelNames(t *testing.T) {
	invalidModelNames := []string{
		"❤️❤️",
		"-modelname",
		"MODELNAME",
		"$$$ModelName",
		"modelName#",
		"トム",
	}

	for _, invalidName := range invalidModelNames {
		t.Run(invalidName, func(t *testing.T) {
			tc.Check(t, IsValidModelName(invalidName), tc.IsFalse)
		})
	}
}

// TestValidModelNames tests a set of known valid model names to make sure
// they pass validation.
func TestValidModelNames(t *testing.T) {
	validModelNames := []string{
		"1",
		"m",
		"m-m",
		"model-",
		"model",
		"009",
	}

	for _, validName := range validModelNames {
		t.Run(validName, func(t *testing.T) {
			tc.Check(t, IsValidModelName(validName), tc.IsTrue)
		})
	}
}
