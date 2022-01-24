// +build !cgo

// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// isErrorRetryable returns true if the given error might be transient and the
// interaction can be safely retried.
func isErrorRetryable(err error) bool {
	return false
}
