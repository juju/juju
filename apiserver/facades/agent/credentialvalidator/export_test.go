// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/facade"
)

func NewCredentialValidatorAPIForTest(st StateAccessor, credentialService CredentialService, resources facade.Resources, authorizer facade.Authorizer) (*CredentialValidatorAPI, error) {
	return internalNewCredentialValidatorAPI(st, credentialService, resources, authorizer, loggo.GetLogger("test"))
}
