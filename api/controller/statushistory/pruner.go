// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"time"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/rpc/params"
)

const apiName = "StatusHistory"

// Client allows calls to "StatusHistory" endpoints.
type Client struct {
	facade base.FacadeCaller
	*common.ModelWatcher
}

// NewClient returns a status "StatusHistory" Client.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, apiName)
	return &Client{facade: facadeCaller, ModelWatcher: common.NewModelWatcher(facadeCaller)}
}

// Prune calls "StatusHistory.Prune"
func (s *Client) Prune(maxHistoryTime time.Duration, maxHistoryMB int) error {
	p := params.StatusHistoryPruneArgs{
		MaxHistoryTime: maxHistoryTime,
		MaxHistoryMB:   maxHistoryMB,
	}
	return s.facade.FacadeCall("Prune", p, nil)
}
