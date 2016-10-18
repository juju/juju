// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"
)

// RetryStrategy holds the necessary information to configure retries.
type RetryStrategy struct {
	ShouldRetry     bool          `json:"should-retry"`
	MinRetryTime    time.Duration `json:"min-retry-time"`
	MaxRetryTime    time.Duration `json:"max-retry-time"`
	JitterRetryTime bool          `json:"jitter-retry-time"`
	RetryTimeFactor int64         `json:"retry-time-factor"`
}

// RetryStrategyResult holds a RetryStrategy or an error.
type RetryStrategyResult struct {
	Error  *Error         `json:"error,omitempty"`
	Result *RetryStrategy `json:"result,omitempty"`
}

// RetryStrategyResults holds the bulk operation result of an API call
// that returns a RetryStrategy or an error.
type RetryStrategyResults struct {
	Results []RetryStrategyResult `json:"results"`
}
