// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func NewCredentialValidatorAPIForTest(
	c *gc.C,
	st StateAccessor,
	cloudService common.CloudService,
	credentialService CredentialService,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*CredentialValidatorAPI, error) {
	return internalNewCredentialValidatorAPI(st, cloudService, credentialService, resources, authorizer, loggertesting.WrapCheckLog(c))
}
