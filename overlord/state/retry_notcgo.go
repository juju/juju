// +build !cgo

// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// IsErrorRetryable returns true if the given error might be transient and the
// interaction can be safely retried.
func IsErrorRetryable(err error) bool {
	return false
}
