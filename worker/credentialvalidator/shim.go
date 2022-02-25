// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/juju/api/agent/credentialvalidator"
	"github.com/juju/juju/api/base"
)

// NewFacade creates a *credentialvalidator.Facade and returns it as a Facade.
func NewFacade(apiCaller base.APICaller) (Facade, error) {
	facade := credentialvalidator.NewFacade(apiCaller)
	return facade, nil
}
