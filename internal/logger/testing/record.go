// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "context"

// RecordLog is a function that records log messages.
type RecordLog func(string, ...any)

// Logf implements logger.Logger.
func (r RecordLog) Logf(msg string, args ...any) {
	r(msg, args)
}

func (r RecordLog) Context() context.Context {
	return context.Background()
}
