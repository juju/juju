// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"time"
)

func NewCSRetryClientForTest(client ResourceClient) *CSRetryClient {
	retryClient := newCSRetryClient(client)
	// Reduce retry delay for test.
	retryClient.retryArgs.Delay = 1 * time.Millisecond
	return retryClient
}
