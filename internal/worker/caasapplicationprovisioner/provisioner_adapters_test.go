// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/juju/tc"

	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/removal"
)

type scaleApplicationService struct {
	ProvisionerApplicationService
	unitUUID coreunit.UUID
}

func (s scaleApplicationService) GetUnitUUID(context.Context, coreunit.Name) (coreunit.UUID, error) {
	return s.unitUUID, nil
}

type scaleRemovalService struct {
	ProvisionerRemovalService
	jobs         []removal.Job
	calls        int
	failFirstJob bool
}

func (s *scaleRemovalService) RemoveUnit(_ context.Context, unitUUID coreunit.UUID, _ bool, _ bool, _ time.Duration) (removal.UUID, error) {
	s.calls++
	s.jobs = append(s.jobs, removal.Job{RemovalType: removal.UnitJob, EntityUUID: unitUUID.String()})
	if s.failFirstJob && s.calls == 1 {
		return "", errors.New("cascaded removal failed")
	}
	return "", nil
}

type provisionerAdaptersSuite struct{}

func TestProvisionerAdapters(t *testing.T) {
	tc.Run(t, &provisionerAdaptersSuite{})
}

func (s *provisionerAdaptersSuite) TestDestroyUnitsRetriesAfterPartialRemoval(c *tc.C) {
	unitUUID := tc.Must(c, coreunit.NewUUID)
	removalService := &scaleRemovalService{failFirstJob: true}
	shim := &provisionerFacadeShim{
		appSvc:     scaleApplicationService{unitUUID: unitUUID},
		removalSvc: removalService,
	}
	err := shim.DestroyUnits(c.Context(), []string{"foo/2"})
	c.Assert(err, tc.ErrorMatches, `destroying unit "foo/2": cascaded removal failed`)
	c.Check(removalService.jobs, tc.HasLen, 1)

	err = shim.DestroyUnits(c.Context(), []string{"foo/2"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removalService.calls, tc.Equals, 2)
}
