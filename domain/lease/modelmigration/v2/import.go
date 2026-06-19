// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v2

import (
	"context"

	"github.com/juju/juju/core/database"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	leasemigration "github.com/juju/juju/domain/lease/modelmigration"
	"github.com/juju/juju/domain/lease/service"
	"github.com/juju/juju/domain/lease/state"
	"github.com/juju/juju/internal/errors"
)

// ImportApplicationLeadership claims a fresh application-leadership lease
// for each leader carried by the envelope. Lease times are not honoured: the
// target always claims a new guarantee window from now.
func ImportApplicationLeadership(
	ctx context.Context, controllerDB database.TxnRunnerFactory, logger logger.Logger,
	modelUUID string, leaders []coremodelmigration.ApplicationLeadership,
) error {
	if len(leaders) == 0 {
		return nil
	}

	leaseSvc := service.NewService(state.NewState(controllerDB, logger))
	for _, l := range leaders {
		key := corelease.Key{ModelUUID: modelUUID, Namespace: corelease.ApplicationLeadershipNamespace, Lease: l.Application}
		req := corelease.Request{Holder: l.Leader, Duration: leasemigration.LeadershipGuarantee}
		if err := leaseSvc.ClaimLease(ctx, key, req); err != nil {
			return errors.Errorf("claiming lease for %q: %w", l.Application, err)
		}
	}
	return nil
}
