// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"context"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/migrationtarget"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type latestLogTimeSuite struct {
	modelMigrationService          *MockModelMigrationService
	modelMigrationServiceGetterErr error
	requestedModelUUID             model.UUID
}

func TestLatestLogTimeSuite(t *testing.T) {
	tc.Run(t, &latestLogTimeSuite{})
}

func (s *latestLogTimeSuite) SetUpTest(c *tc.C) {
	s.modelMigrationService = NewMockModelMigrationService(gomock.NewController(c))
	s.modelMigrationServiceGetterErr = nil
	s.requestedModelUUID = ""
}

func (s *latestLogTimeSuite) api(c *tc.C) *migrationtarget.API {
	api, err := migrationtarget.NewAPI(
		// LatestLogTime only uses the model migration service getter
		// and the logger, so the remaining dependencies can be nil.
		&facadetest.ModelContext{},
		nil,
		nil, nil, nil, nil, nil, nil, nil,
		nil,
		func(_ context.Context, modelUUID model.UUID) (migrationtarget.ModelMigrationService, error) {
			s.requestedModelUUID = modelUUID
			return s.modelMigrationService, s.modelMigrationServiceGetterErr
		},
		nil,
		facades.FacadeVersions{},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
	return api
}

func (s *latestLogTimeSuite) latestLogTime(c *tc.C, modelUUID string) (time.Time, error) {
	return s.api(c).LatestLogTime(
		c.Context(),
		params.ModelArgs{ModelTag: names.NewModelTag(modelUUID).String()},
	)
}

func (s *latestLogTimeSuite) TestLatestLogTime(c *tc.C) {
	lastTime := time.Date(2024, 2, 18, 6, 23, 24, 0, time.UTC)
	modelUUID := model.UUID(uuid.MustNewUUID().String())

	s.modelMigrationService.EXPECT().LastLogTransferTime(gomock.Any()).Return(lastTime, nil)

	latest, err := s.latestLogTime(c, modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(latest, tc.Equals, lastTime)
	c.Check(s.requestedModelUUID, tc.Equals, modelUUID)
}

func (s *latestLogTimeSuite) TestLatestLogTimeNotATag(c *tc.C) {
	_, err := s.api(c).LatestLogTime(c.Context(), params.ModelArgs{ModelTag: "not-a-tag"})
	c.Assert(err, tc.ErrorMatches, `cannot parse model tag: "not-a-tag" is not a valid tag`)
}

func (s *latestLogTimeSuite) TestLatestLogTimeServiceError(c *tc.C) {
	s.modelMigrationService.EXPECT().LastLogTransferTime(gomock.Any()).
		Return(time.Time{}, errors.New("boom"))

	_, err := s.latestLogTime(c, uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorMatches, ".*cannot get last log transfer time for model .*: boom")
}

func (s *latestLogTimeSuite) TestLatestLogTimeServiceGetterError(c *tc.C) {
	s.modelMigrationServiceGetterErr = errors.New("boom")

	_, err := s.latestLogTime(c, uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorMatches, ".*cannot get model migration service for model .*: boom")
}
