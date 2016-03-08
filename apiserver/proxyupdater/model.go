// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("ProxyUpdater", 1, NewProxyUpdaterAPI)
}

// ProxyUpdaterAPI implements the API used by the proxy updater worker.
type ProxyUpdaterAPI struct {
	*common.ModelWatcher
}

// NewProxyUpdaterAPI creates a new instance of the ProxyUpdater API.
func NewProxyUpdaterAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*ProxyUpdaterAPI, error) {
	return &ProxyUpdaterAPI{
		ModelWatcher: common.NewModelWatcher(st, resources, authorizer),
	}, nil
}
