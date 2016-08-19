// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api/client"
	"github.com/juju/juju/resource/api/server"
)

// NewAPIClient is mostly a copy of the newClient code in
// component/all/resources.go.  It lives here because it simplifies this code
// immensely.
func NewAPIClient(apiCaller base.APICallCloser) (*client.Client, error) {
	caller := base.NewFacadeCallerForVersion(apiCaller, resource.FacadeName, server.Version)

	httpClient, err := apiCaller.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// The apiCaller takes care of prepending /environment/<envUUID>.
	apiClient := client.NewClient(caller, httpClient, apiCaller)
	return apiClient, nil
}
