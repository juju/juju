// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/uniter"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	jujufactory "github.com/juju/juju/testing/factory"
)

//TODO run all common V0 and V1 tests.
type uniterV2Suite struct {
	uniterBaseSuite
	uniter *uniter.UniterAPIV2
}

var _ = gc.Suite(&uniterV2Suite{})

func (s *uniterV2Suite) SetUpTest(c *gc.C) {
	s.uniterBaseSuite.setUpTest(c)

	uniterAPIV2, err := uniter.NewUniterAPIV2(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.uniter = uniterAPIV2
}

func (s *uniterV2Suite) TestStorageInstances(c *gc.C) {
	// We need to set up a unit that has storage metadata defined.
	ch := s.AddTestingCharm(c, "storage-block")
	sCons := map[string]state.StorageConstraints{
		"data": state.StorageConstraints{Pool: "", Size: 1024, Count: 1},
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block", ch, sCons)
	factory := jujufactory.NewFactory(s.State)
	unit := factory.MakeUnit(c, &jujufactory.UnitParams{
		Service: service,
	})

	password, err := utils.RandomPassword()
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	st := s.OpenAPIAs(c, unit.Tag(), password)
	uniter, err := st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	instances, err := uniter.StorageInstances(unit.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.DeepEquals, []storage.StorageInstance{
		{Id: "data/0", Kind: storage.StorageKindBlock, Location: ""},
	})
}
