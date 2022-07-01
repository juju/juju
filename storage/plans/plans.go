// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plans

import (
	"github.com/juju/errors"

	"github.com/juju/juju/v3/storage"
	"github.com/juju/juju/v3/storage/plans/common"
	"github.com/juju/juju/v3/storage/plans/iscsi"
	"github.com/juju/juju/v3/storage/plans/local"
)

var registry = map[storage.DeviceType]common.Plan{
	storage.DeviceTypeLocal: local.NewLocalPlan(),
	storage.DeviceTypeISCSI: iscsi.NewiSCSIPlan(),
}

func PlanByType(name storage.DeviceType) (common.Plan, error) {
	plan, ok := registry[name]
	if !ok {
		return nil, errors.NotFoundf("plan type %s not found", name)
	}
	return plan, nil
}
