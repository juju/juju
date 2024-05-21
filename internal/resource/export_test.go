// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"time"

	"github.com/juju/names/v5"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/state"
)

func NewCSRetryClientForTest(client ResourceGetter) *ResourceRetryClient {
	retryClient := newRetryClient(client)
	// Reduce retry delay for test.
	retryClient.retryArgs.Delay = 1 * time.Millisecond
	return retryClient
}

func NewCharmHubClientForTest(cl CharmHub, logger corelogger.Logger) *CharmHubClient {
	return &CharmHubClient{
		client: cl,
		logger: logger,
	}
}

func NewResourceRetryClientForTest(cl ResourceGetter) *ResourceRetryClient {
	client := newRetryClient(cl)
	client.retryArgs.Delay = time.Millisecond
	return client
}

func NewResourceOpenerForTest(
	res Resources,
	tag names.Tag,
	unitName string,
	appName string,
	charmURL *charm.URL,
	charmOrigin state.CharmOrigin,
	resourceClient ResourceGetter,
	resourceDownloadLimiter ResourceDownloadLock,
) *ResourceOpener {
	return &ResourceOpener{
		modelUUID:      "uuid",
		resourceCache:  res,
		user:           tag,
		unitName:       unitName,
		appName:        appName,
		charmURL:       charmURL,
		charmOrigin:    charmOrigin,
		resourceClient: resourceClient,
		resourceDownloadLimiterFunc: func() ResourceDownloadLock {
			return resourceDownloadLimiter
		},
	}
}
