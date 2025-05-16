// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common/crossmodel"
	mocks "github.com/juju/juju/apiserver/common/crossmodel/mocks"
	corecrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type crossmodelSuite struct {
	coretesting.BaseSuite

	statusService *mocks.MockStatusService
}

func TestCrossmodelSuite(t *stdtesting.T) { tc.Run(t, &crossmodelSuite{}) }
func (s *crossmodelSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.statusService = mocks.NewMockStatusService(ctrl)
	return ctrl
}

func (s *crossmodelSuite) TestExpandChangeWhenRelationHasGone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Other aspects of ExpandChange are tested in the
	// crossmodelrelations and remoterelations facade tests.
	change := params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"app/0": {Version: 1234},
		},
		AppChanged: map[string]int64{
			"app": 3456,
		},
		Departed: []string{"app/2", "app/3"},
	}
	result, err := crossmodel.ExpandChange(
		&mockBackend{}, "some-relation", "some-app", change)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.RemoteRelationChangeEvent{
		RelationToken:           "some-relation",
		ApplicationOrOfferToken: "some-app",
		DepartedUnits:           []int{2, 3},
	})
}

func (s *crossmodelSuite) TestGetOfferStatusChangeOfferGoneNotMigrating(c *tc.C) {
	defer s.setupMocks(c).Finish()

	st := &mockBackend{}
	ch, err := crossmodel.GetOfferStatusChange(c.Context(), st, s.statusService, "uuid", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch, tc.DeepEquals, &params.OfferStatusChange{
		OfferName: "mysql",
		Status:    params.EntityStatus{Status: status.Terminated, Info: "offer has been removed"},
	})
}

func (s *crossmodelSuite) TestGetOfferStatusChangeOfferGoneMigrating(c *tc.C) {
	defer s.setupMocks(c).Finish()

	st := &mockBackend{
		migrating: true,
	}

	_, err := crossmodel.GetOfferStatusChange(c.Context(), st, s.statusService, "uuid", "mysql")
	c.Assert(err, tc.ErrorMatches, "model is being migrated")
}

func (s *crossmodelSuite) TestGetOfferStatusChangeApplicationGoneNotMigrating(c *tc.C) {
	defer s.setupMocks(c).Finish()

	st := &mockBackend{
		appName: "mysql-app",
	}
	s.statusService.EXPECT().GetApplicationDisplayStatus(gomock.Any(), "mysql-app").Return(status.StatusInfo{}, statuserrors.ApplicationNotFound)

	ch, err := crossmodel.GetOfferStatusChange(c.Context(), st, s.statusService, "deadbeef", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch, tc.DeepEquals, &params.OfferStatusChange{
		OfferName: "mysql",
		Status:    params.EntityStatus{Status: status.Terminated, Info: "application has been removed"},
	})
}

func (s *crossmodelSuite) TestGetOfferStatusChangeApplicationGoneMigrating(c *tc.C) {
	defer s.setupMocks(c).Finish()

	st := &mockBackend{
		migrating: true,
		appName:   "mysql-app",
	}
	s.statusService.EXPECT().GetApplicationDisplayStatus(gomock.Any(), "mysql-app").Return(status.StatusInfo{}, statuserrors.ApplicationNotFound)

	_, err := crossmodel.GetOfferStatusChange(c.Context(), st, s.statusService, "deadbeef", "mysql")
	c.Assert(err, tc.ErrorMatches, "model is being migrated")
}

func (s *crossmodelSuite) TestGetOfferStatusChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.statusService.EXPECT().GetApplicationDisplayStatus(gomock.Any(), "mysql-app").Return(status.StatusInfo{Status: status.Active}, nil)

	st := &mockBackend{appName: "mysql-app"}
	ch, err := crossmodel.GetOfferStatusChange(c.Context(), st, s.statusService, "deadbeef", "mysql")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch, tc.DeepEquals, &params.OfferStatusChange{
		OfferName: "mysql",
		Status:    params.EntityStatus{Status: status.Active},
	})
}

type mockBackend struct {
	testhelpers.Stub
	crossmodel.Backend

	appName        string
	remoteEntities map[names.Tag]string
	migrating      bool
}

func (st *mockBackend) GetRemoteEntity(token string) (names.Tag, error) {
	st.MethodCall(st, "GetRemoteEntity", token)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	for e, t := range st.remoteEntities {
		if t == token {
			return e, nil
		}
	}
	return nil, errors.NotFoundf("token %v", token)
}

func (st *mockBackend) ApplicationOfferForUUID(uuid string) (*corecrossmodel.ApplicationOffer, error) {
	if uuid != "deadbeef" {
		return nil, errors.NotFoundf(uuid)
	}
	return &corecrossmodel.ApplicationOffer{
		ApplicationName: st.appName,
	}, nil
}

func (st *mockBackend) IsMigrationActive() (bool, error) {
	return st.migrating, nil
}
