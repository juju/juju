// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"context"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Client provides access to a worker's view of the state.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a version of the state that provides functionality required by the worker.
func NewClient(caller base.APICaller) *Client {
	return &Client{base.NewFacadeCaller(caller, "CharmRevisionUpdater")}
}

// UpdateLatestRevisions retrieves charm revision info from a repository
// and updates the revision info in state.
func (st *Client) UpdateLatestRevisions() error {
	result := new(params.ErrorResult)
	err := st.facade.FacadeCall(context.TODO(), "UpdateLatestRevisions", nil, result)
	if err != nil {
		return err
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}
