// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"time"

	"github.com/juju/names/v4"

	"github.com/juju/juju/resource/respositories"
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

func NewResourceRetryClientForTest(cl respositories.ResourceGetter) *ResourceRetryClient {
	return newRetryClient(cl)
}

func NewResourceOpenerForTest(
	st ResourceOpenerState,
	res Resources,
	tag names.Tag,
	unit Unit,
	fn func(st ResourceOpenerState) ResourceRetryClientGetter,
) *ResourceOpener {
	return &ResourceOpener{
		st:                st,
		res:               res,
		userID:            tag,
		unit:              unit,
		newResourceOpener: fn,
	}
}
