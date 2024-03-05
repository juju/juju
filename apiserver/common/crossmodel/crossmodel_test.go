// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/crossmodel"
	corecrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type crossmodelSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&crossmodelSuite{})

func (s *crossmodelSuite) TestExpandChangeWhenRelationHasGone(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RemoteRelationChangeEvent{
		RelationToken:           "some-relation",
		ApplicationOrOfferToken: "some-app",
		DepartedUnits:           []int{2, 3},
	})
}

func (s *crossmodelSuite) TestGetOfferStatusChangeOfferGoneNotMigrating(c *gc.C) {
	st := &mockBackend{}
	ch, err := crossmodel.GetOfferStatusChange(st, "uuid", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, gc.DeepEquals, &params.OfferStatusChange{
		OfferName: "mysql",
		Status:    params.EntityStatus{Status: status.Terminated, Info: "offer has been removed"},
	})
}

func (s *crossmodelSuite) TestGetOfferStatusChangeOfferGoneMigrating(c *gc.C) {
	st := &mockBackend{
		migrating: true,
	}

	_, err := crossmodel.GetOfferStatusChange(st, "uuid", "mysql")
	c.Assert(err, gc.ErrorMatches, "model is being migrated")
}

func (s *crossmodelSuite) TestGetOfferStatusChangeApplicationGoneNotMigrating(c *gc.C) {
	st := &mockBackend{}
	ch, err := crossmodel.GetOfferStatusChange(st, "deadbeef", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, gc.DeepEquals, &params.OfferStatusChange{
		OfferName: "mysql",
		Status:    params.EntityStatus{Status: status.Terminated, Info: "application has been removed"},
	})
}

func (s *crossmodelSuite) TestGetOfferStatusChangeApplicationGoneMigrating(c *gc.C) {
	st := &mockBackend{
		migrating: true,
	}

	_, err := crossmodel.GetOfferStatusChange(st, "deadbeef", "mysql")
	c.Assert(err, gc.ErrorMatches, "model is being migrated")
}

func (s *crossmodelSuite) TestGetOfferStatusChange(c *gc.C) {
	st := &mockBackend{appName: "mysql", appStatus: status.StatusInfo{Status: status.Active}}
	ch, err := crossmodel.GetOfferStatusChange(st, "deadbeef", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, gc.DeepEquals, &params.OfferStatusChange{
		OfferName: "mysql",
		Status:    params.EntityStatus{Status: status.Active},
	})
}

type mockBackend struct {
	testing.Stub
	crossmodel.Backend

	appName        string
	appStatus      status.StatusInfo
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

func (st *mockBackend) Application(name string) (crossmodel.Application, error) {
	if name != "mysql" {
		return nil, errors.NotFoundf(name)
	}
	return &mockApplication{
		name:   "mysql",
		status: st.appStatus,
	}, nil
}

func (st *mockBackend) IsMigrationActive() (bool, error) {
	return st.migrating, nil
}

type mockApplication struct {
	crossmodel.Application
	name   string
	status status.StatusInfo
}

func (a *mockApplication) Name() string {
	return a.name
}

func (a *mockApplication) Status() (status.StatusInfo, error) {
	return a.status, nil
}
