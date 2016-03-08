// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"
)

// RetryStrategy holds the necessary information to configure retries.
type RetryStrategy struct {
	ShouldRetry     bool
	MinRetryTime    time.Duration
	MaxRetryTime    time.Duration
	JitterRetryTime bool
	RetryTimeFactor int64
}

// RetryStrategyResult holds a RetryStrategy or an error.
type RetryStrategyResult struct {
	Error  *Error
	Result *RetryStrategy
}

// RetryStrategyResults holds the bulk operation result of an API call
// that returns a RetryStrategy or an error.
type RetryStrategyResults struct {
	Results []RetryStrategyResult
}
