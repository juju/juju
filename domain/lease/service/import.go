// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/lease"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

// DeleteLeadershipForModel deletes all application-leadership leases for the
// given model. It is idempotent: if no leases exist, it returns nil.
func (s *Service) DeleteLeadershipForModel(ctx context.Context, modelUUID coremodel.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("validating model uuid: %w", err)
	}
	return s.st.DeleteLeadershipForModel(ctx, modelUUID.String())
}

// LeadershipGuarantee is the amount of time that the lease service will
// guarantee that the application leader will be the holder of the lease.
const LeadershipGuarantee = time.Minute

// ImportApplicationLeadership claims a fresh application-leadership lease for
// each leader carried by the v8 migration envelope. Lease times are not
// honoured: the target always claims a new guarantee window from now.
//
// It is called directly by the v8 migration import driver in
// internal/migration.
func (s *Service) ImportApplicationLeadership(
	ctx context.Context, modelUUID coremodel.UUID, leaders []coremodelmigration.ApplicationLeadership,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(leaders) == 0 {
		return nil
	}

	for _, l := range leaders {
		key := lease.Key{
			ModelUUID: modelUUID.String(),
			Namespace: lease.ApplicationLeadershipNamespace,
			Lease:     l.Application,
		}
		req := lease.Request{Holder: l.Leader, Duration: LeadershipGuarantee}
		if err := s.ClaimLease(ctx, key, req); err != nil {
			return errors.Errorf("claiming lease for %q: %w", l.Application, err)
		}
	}
	return nil
}
