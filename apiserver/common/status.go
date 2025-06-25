// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
)

// EntityStatusFromState converts a state.StatusInfo into a params.EntityStatus.
func EntityStatusFromState(statusInfo status.StatusInfo) params.EntityStatus {
	return params.EntityStatus{
		Status: statusInfo.Status,
		Info:   statusInfo.Message,
		Data:   statusInfo.Data,
		Since:  statusInfo.Since,
	}
}
