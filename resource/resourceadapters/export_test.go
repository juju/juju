// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"time"

	"github.com/juju/names/v4"

	"github.com/juju/juju/resource/repositories"
)

func NewCSRetryClientForTest(client ResourceClient) *ResourceRetryClient {
	retryClient := newRetryClient(client)
	// Reduce retry delay for test.
	retryClient.retryArgs.Delay = 1 * time.Millisecond
	return retryClient
}

func NewCharmHubClientForTest(cl CharmHub, logger Logger) *CharmHubClient {
	return &CharmHubClient{
		client: cl,
		logger: logger,
	}
}

func NewResourceRetryClientForTest(cl repositories.ResourceGetter) *ResourceRetryClient {
	return newRetryClient(cl)
}

func NewResourceOpenerForTest(
	st ResourceOpenerState,
	res Resources,
	tag names.Tag,
	unit Unit,
	application Application,
	fn func(st ResourceOpenerState) ResourceRetryClientGetter,
	maxRequests int,
) *ResourceOpener {
	return &ResourceOpener{
		st:                st,
		res:               res,
		userID:            tag,
		unit:              unit,
		application:       application,
		newResourceOpener: fn,
		resourceDownloadLimiterFunc: func() ResourceDownloadLock {
			return NewResourceDownloadLimiter(maxRequests, 0)
		},
	}
}
