// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/juju/apiserver/facade"
)

func NewCredentialValidatorAPIForTest(b Backend, watcherRegistry facade.WatcherRegistry, authorizer facade.Authorizer) (*CredentialValidatorAPI, error) {
	return internalNewCredentialValidatorAPI(b, watcherRegistry, authorizer)
}
