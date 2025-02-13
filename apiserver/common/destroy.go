// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import "time"

// MaxWait is how far in the future the backstop force cleanup will be scheduled.
// Default is 1min if no value is provided.
func MaxWait(in *time.Duration) time.Duration {
	if in != nil {
		return *in
	}
	return 1 * time.Minute
}
