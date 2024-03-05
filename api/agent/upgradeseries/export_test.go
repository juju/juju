// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
)

func NewStateFromCaller(caller base.FacadeCaller, authTag names.Tag) *Client {
	return &Client{
		facade:           caller,
		authTag:          authTag,
		UpgradeSeriesAPI: common.NewUpgradeSeriesAPI(caller, authTag),
	}
}
