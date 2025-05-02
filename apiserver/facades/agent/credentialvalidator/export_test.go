// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"context"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func NewCredentialValidatorAPIForTest(
	c *gc.C,
	cloudService CloudService,
	credentialService CredentialService,
	modelService ModelService,
	modelInfoService ModelInfoService,
	modelCredentialWatcher func(ctx context.Context) (watcher.NotifyWatcher, error),
	watcherRegistry facade.WatcherRegistry,
) *CredentialValidatorAPI {
	return internalNewCredentialValidatorAPI(cloudService, credentialService, modelService, modelInfoService, modelCredentialWatcher, watcherRegistry, loggertesting.WrapCheckLog(c))

}
