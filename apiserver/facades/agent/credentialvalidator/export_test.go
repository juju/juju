// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
)

func NewCredentialValidatorAPIForTest(
	st StateAccessor, cloudService common.CloudService, credentialService CredentialService, resources facade.Resources, authorizer facade.Authorizer,
) (*CredentialValidatorAPI, error) {
	return internalNewCredentialValidatorAPI(st, cloudService, credentialService, resources, authorizer, loggo.GetLogger("test"))
}
