// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"time"

	coreapplication "github.com/juju/juju/core/application"
	corelogger "github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/state"
)

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
	unitName coreunit.Name,
	unitUUID coreunit.UUID,
	appName string,
	appID coreapplication.ID,
	resourceService ResourceService,
	charmURL *charm.URL,
	charmOrigin state.CharmOrigin,
	resourceClientGetter resourceClientGetterFunc,
	resourceDownloadLimiter ResourceDownloadLock,
) *ResourceOpener {
	var (
		retrievedBy     string
		retrievedByType resource.RetrievedByType
		setResourceFunc func(ctx context.Context, resourceUUID coreresource.UUID) error
	)
	if unitName.String() != "" {
		retrievedBy = unitName.String()
		retrievedByType = resource.Unit
		setResourceFunc = func(ctx context.Context, resourceUUID coreresource.UUID) error {
			return resourceService.SetUnitResource(ctx, resourceUUID, unitUUID)
		}
	} else {
		retrievedBy = appName
		retrievedByType = resource.Application
		setResourceFunc = resourceService.SetApplicationResource
	}
	return &ResourceOpener{
		modelUUID:            "uuid",
		resourceService:      resourceService,
		retrievedBy:          retrievedBy,
		retrievedByType:      retrievedByType,
		unitName:             unitName,
		appName:              appName,
		appID:                appID,
		charmURL:             charmURL,
		charmOrigin:          charmOrigin,
		resourceClientGetter: resourceClientGetter,
		resourceDownloadLimiterFunc: func() ResourceDownloadLock {
			return resourceDownloadLimiter
		},
		setResourceFunc: setResourceFunc,
	}
}
