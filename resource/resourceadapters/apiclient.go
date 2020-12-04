// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/resources/client"
	"github.com/juju/juju/resource"
)

// NewAPIClient is mostly a copy of the newClient code in
// component/all/resources.go.  It lives here because it simplifies this code
// immensely.
func NewAPIClient(apiCaller base.APICallCloser) (*client.Client, error) {
	caller := base.NewFacadeCaller(apiCaller, resource.FacadeName)

	httpClient, err := apiCaller.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// The apiCaller takes care of prepending /environment/<modelUUID>.
	apiClient := client.NewClient(apiCaller.Context(), caller, httpClient, apiCaller)
	return apiClient, nil
}
