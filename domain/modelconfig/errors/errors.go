// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
)

// PartialSaveError represents a specific error that has occurred during the
// save operation if not all model config attributes could be written.
// This error typically occurs if an OnSave handler errors out.
type PartialSaveError struct {
	// ErrorAttrs is the list of attributes in the config that could not
	// be saved, if known.
	ErrorAttrs []string

	// Cause is the reason for why the save failed.
	Cause error
}

// Error implements Error interface.
func (e PartialSaveError) Error() string {
	if e.Cause == nil && len(e.ErrorAttrs) == 0 {
		return "error saving config: only some attributes are set"
	}
	if e.Cause != nil && len(e.ErrorAttrs) > 0 {
		return fmt.Sprintf("error saving config attributes %v because %s", e.ErrorAttrs, e.Cause)

	}
	if len(e.ErrorAttrs) > 0 {
		return fmt.Sprintf("error saving config attributes %v", e.ErrorAttrs)
	}
	return fmt.Sprintf("error saving config attributes: %s", e.Cause)
}

func (e PartialSaveError) Unwrap() error {
	return e.Cause
}
