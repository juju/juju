// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd

import (
	"reflect"
)

// Audit holds audit information about an externally-initiated
// operation in the controller/model.
type Audit struct {
	// Operation identifies the actual audited operation.
	Operation string

	// Args are the arguments used for the operation.
	Args map[string]string
}

// IsZero indicates whether or not it is the zero value.
func (audit Audit) IsZero() bool {
	return reflect.DeepEqual(audit, Audit{})
}
