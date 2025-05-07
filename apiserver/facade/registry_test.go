// Copyright 2014-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade_test

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/testing"
)

type RegistrySuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&RegistrySuite{})

var (
	interfaceType = reflect.TypeOf((*interface{})(nil)).Elem()
	intPtrType    = reflect.TypeOf((*int)(nil))
)

func (s *RegistrySuite) TestRegister(c *tc.C) {
	registry := &facade.Registry{}
	err := registry.Register("myfacade", 123, testFacade, interfaceType)
	c.Assert(err, jc.ErrorIsNil)

	factory, err := registry.GetFactory("myfacade", 123)
	c.Assert(err, jc.ErrorIsNil)
	val, err := factory(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(val, tc.Equals, "myobject")
}

func (s *RegistrySuite) TestRegisterForMultiModel(c *tc.C) {
	registry := &facade.Registry{}
	err := registry.RegisterForMultiModel("myfacade", 123, testFacadeModel, interfaceType)
	c.Assert(err, jc.ErrorIsNil)

	factory, err := registry.GetFactory("myfacade", 123)
	c.Assert(err, jc.ErrorIsNil)
	val, err := factory(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(val, tc.Equals, "myobject")
}

func (s *RegistrySuite) TestListDetails(c *tc.C) {
	registry := &facade.Registry{}
	err := registry.Register("f2", 6, testFacade, interfaceType)
	c.Assert(err, jc.ErrorIsNil)

	err = registry.Register("f1", 9, validIdFactory, intPtrType)
	c.Assert(err, jc.ErrorIsNil)

	details := registry.ListDetails()
	c.Assert(details, tc.HasLen, 2)
	c.Assert(details[0].Name, tc.Equals, "f1")
	c.Assert(details[0].Version, tc.Equals, 9)
	v, _ := details[0].Factory(context.Background(), nil)
	c.Assert(v, tc.FitsTypeOf, new(int))
	c.Assert(details[0].Type, tc.Equals, intPtrType)

	c.Assert(details[1].Name, tc.Equals, "f2")
	c.Assert(details[1].Version, tc.Equals, 6)
	v, _ = details[1].Factory(context.Background(), nil)
	c.Assert(v, tc.Equals, "myobject")
	c.Assert(details[1].Type, tc.Equals, interfaceType)
}

func (*RegistrySuite) TestGetFactoryUnknown(c *tc.C) {
	registry := &facade.Registry{}
	factory, err := registry.GetFactory("name", 0)
	c.Check(err, jc.ErrorIs, errors.NotFound)
	c.Check(err, tc.ErrorMatches, `name\(0\) not found`)
	c.Check(factory, tc.IsNil)
}

func (*RegistrySuite) TestGetFactoryUnknownVersion(c *tc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)

	factory, err := registry.GetFactory("name", 1)
	c.Check(err, jc.ErrorIs, errors.NotFound)
	c.Check(err, tc.ErrorMatches, `name\(1\) not found`)
	c.Check(factory, tc.IsNil)
}

func (*RegistrySuite) TestRegisterAndList(c *tc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)

	c.Check(registry.List(), jc.DeepEquals, []facade.Description{
		{Name: "name", Versions: []int{0}},
	})
}

func (*RegistrySuite) TestRegisterAndListSorted(c *tc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 10)
	assertRegister(c, registry, "name", 0)
	assertRegister(c, registry, "name", 101)

	c.Check(registry.List(), jc.DeepEquals, []facade.Description{
		{Name: "name", Versions: []int{0, 10, 101}},
	})
}

func (*RegistrySuite) TestRegisterAndListMultiple(c *tc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "other", 0)
	assertRegister(c, registry, "name", 0)
	assertRegister(c, registry, "third", 2)
	assertRegister(c, registry, "third", 3)

	c.Check(registry.List(), jc.DeepEquals, []facade.Description{
		{Name: "name", Versions: []int{0}},
		{Name: "other", Versions: []int{0}},
		{Name: "third", Versions: []int{2, 3}},
	})
}

func (*RegistrySuite) TestRegisterAlreadyPresent(c *tc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)
	secondIdFactory := func(_ context.Context, context facade.ModelContext) (facade.Facade, error) {
		var i = 200
		return &i, nil
	}
	err := registry.Register("name", 0, secondIdFactory, intPtrType)
	c.Assert(err, tc.ErrorMatches, `object "name\(0\)" already registered`)

	factory, err := registry.GetFactory("name", 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(factory, tc.NotNil)
	val, err := factory(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	asIntPtr := val.(*int)
	c.Check(*asIntPtr, tc.Equals, 100)
}

func (*RegistrySuite) TestGetFactory(c *tc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)

	factory, err := registry.GetFactory("name", 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(factory, tc.NotNil)
	res, err := factory(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, tc.NotNil)
	asIntPtr := res.(*int)
	c.Check(*asIntPtr, tc.Equals, 100)
}

func (*RegistrySuite) TestGetType(c *tc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)

	typ, err := registry.GetType("name", 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(typ, tc.Equals, intPtrType)
}

func (*RegistrySuite) TestDiscardHandlesNotPresent(c *tc.C) {
	registry := &facade.Registry{}
	registry.Discard("name", 1)
}

func (*RegistrySuite) TestDiscardRemovesEntry(c *tc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)
	_, err := registry.GetFactory("name", 0)
	c.Assert(err, jc.ErrorIsNil)

	registry.Discard("name", 0)
	factory, err := registry.GetFactory("name", 0)
	c.Check(err, jc.ErrorIs, errors.NotFound)
	c.Check(err, tc.ErrorMatches, `name\(0\) not found`)
	c.Check(factory, tc.IsNil)
	c.Check(registry.List(), jc.DeepEquals, []facade.Description{})
}

func (*RegistrySuite) TestDiscardLeavesOtherVersions(c *tc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)
	assertRegister(c, registry, "name", 1)

	registry.Discard("name", 0)
	_, err := registry.GetFactory("name", 1)
	c.Check(err, jc.ErrorIsNil)
	c.Check(registry.List(), jc.DeepEquals, []facade.Description{
		{Name: "name", Versions: []int{1}},
	})
}

func assertRegister(c *tc.C, registry *facade.Registry, name string, version int) {
	assertRegisterFlag(c, registry, name, version)
}

func assertRegisterFlag(c *tc.C, registry *facade.Registry, name string, version int) {
	err := registry.Register(name, version, validIdFactory, intPtrType)
	c.Assert(err, tc.IsNil)
}

func testFacade(_ context.Context, _ facade.ModelContext) (facade.Facade, error) {
	return "myobject", nil
}

func testFacadeModel(_ context.Context, _ facade.MultiModelContext) (facade.Facade, error) {
	return "myobject", nil
}

func validIdFactory(_ context.Context, _ facade.ModelContext) (facade.Facade, error) {
	var i = 100
	return &i, nil
}
