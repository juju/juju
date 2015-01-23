// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/storage"
	"github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type storageSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite

	api        *storage.API
	authorizer testing.FakeAuthorizer
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authorizer = testing.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.api, err = storage.NewAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageSuite) TestShowStorage(c *gc.C) {
	unit := s.createTestUnit(c)
	wanted := params.StorageInstance{UnitName: unit.Name(), StorageName: "test-storage"}

	found, err := s.api.Show(wanted)
	c.Assert(err.Error(), gc.Matches, ".*not implemented.*")
	c.Assert(found.Results, gc.HasLen, 0)
	c.Assert(found.Results, gc.DeepEquals, []params.StorageInstance{})
}

func (s *storageSuite) createTestUnit(c *gc.C) *state.Unit {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	wordpress := s.Factory.MakeService(c, &factory.ServiceParams{
		Charm: charm,
	})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Service: wordpress,
		Machine: machine,
	})
	return unit
}
