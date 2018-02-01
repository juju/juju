// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport

// dialRequestTimeoutError is an error type used when
// sending a dial request times out.
type dialRequestTimeoutError struct{}

func (dialRequestTimeoutError) Error() string {
	return "timed out dialing"
}

func (dialRequestTimeoutError) Temporary() bool {
	return true
}

func (dialRequestTimeoutError) Timeout() bool {
	return true
}

// dialWorkerStoppedError wraps an error that indicates
// the dial worker has stopped as the reason why dialling
// failed.
type dialWorkerStoppedError struct {
	error
}

func (dialWorkerStoppedError) Temporary() bool {
	return true
}

func (dialWorkerStoppedError) Timeout() bool {
	return false
}
