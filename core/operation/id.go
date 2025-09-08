// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

// ID is a unique name identifier for an operation.
type ID string

// String returns the [ID] as a string.
func (i ID) String() string {
	return string(i)
}
