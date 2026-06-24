// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/lease"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type importSuite struct {
	testhelpers.IsolationSuite

	state *MockState
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *importSuite) TestImportApplicationLeadership(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must0(c, coremodel.NewUUID)
	s.state.EXPECT().ClaimLease(gomock.Any(), gomock.Any(), lease.Key{
		ModelUUID: modelUUID.String(),
		Namespace: lease.ApplicationLeadershipNamespace,
		Lease:     "postgresql",
	}, lease.Request{Holder: "postgresql/0", Duration: LeadershipGuarantee}).Return(nil)
	s.state.EXPECT().ClaimLease(gomock.Any(), gomock.Any(), lease.Key{
		ModelUUID: modelUUID.String(),
		Namespace: lease.ApplicationLeadershipNamespace,
		Lease:     "mysql",
	}, lease.Request{Holder: "mysql/1", Duration: LeadershipGuarantee}).Return(nil)

	err := NewService(s.state).ImportApplicationLeadership(c.Context(), modelUUID, []coremodelmigration.ApplicationLeadership{
		{Application: "postgresql", Leader: "postgresql/0"},
		{Application: "mysql", Leader: "mysql/1"},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportApplicationLeadershipEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).ImportApplicationLeadership(c.Context(), tc.Must0(c, coremodel.NewUUID), nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportApplicationLeadershipError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := errors.New("boom")
	s.state.EXPECT().ClaimLease(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(expected)

	err := NewService(s.state).ImportApplicationLeadership(
		c.Context(), tc.Must0(c, coremodel.NewUUID),
		[]coremodelmigration.ApplicationLeadership{{Application: "postgresql", Leader: "postgresql/0"}},
	)
	c.Assert(err, tc.ErrorIs, expected)
}
