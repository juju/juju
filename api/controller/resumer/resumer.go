// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer

import (
	"github.com/juju/juju/api/base"
)

const resumerFacade = "Resumer"

// API provides access to the Resumer API facade.
type API struct {
	facade base.FacadeCaller
}

// NewAPI creates a new client-side Resumer facade.
func NewAPI(caller base.APICaller) *API {
	facadeCaller := base.NewFacadeCaller(caller, resumerFacade)
	return &API{facade: facadeCaller}

}

// ResumeTransactions calls the server-side ResumeTransactions method.
func (api *API) ResumeTransactions() error {
	return api.facade.FacadeCall("ResumeTransactions", nil, nil)
}
