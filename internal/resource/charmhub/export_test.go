// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	corelogger "github.com/juju/juju/core/logger"
)

func NewCharmHubClientForTest(cl CharmHub, logger corelogger.Logger) *CharmHubClient {
	return &CharmHubClient{
		client: cl,
		logger: logger,
	}
}
