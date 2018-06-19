// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager

import (
	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
)

func NewCredentialManagerAPIForTest(b credentialcommon.StateBackend, resources facade.Resources, authorizer facade.Authorizer) (*CredentialManagerAPI, error) {
	return internalNewCredentialManagerAPI(b, resources, authorizer)
}
