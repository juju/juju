// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import "time"

const (
	defaultMaxAllocs   = 10 * 1024 * 1024
	defaultMaxSteps    = 100_000
	defaultMaxDuration = 5 * time.Second
)
