// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	corelogger "github.com/juju/juju/core/logger"
)

func NewCharmHubClientForTest(cl CharmHub, downloader Downloader, logger corelogger.Logger) *CharmHubClient {
	return &CharmHubClient{
		downloader: downloader,
		client:     cl,
		logger:     logger,
	}
}
