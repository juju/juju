// Copyright 2014-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade_test

import (
	"reflect"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/testing"
)

type RegistrySuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&RegistrySuite{})

func (s *RegistrySuite) TestRegister(c *gc.C) {
	registry := &facade.Registry{}
	var v interface{}
	facadeType := reflect.TypeOf(&v).Elem()
	err := registry.Register("myfacade", 123, testFacade, facadeType, "")
	c.Assert(err, jc.ErrorIsNil)

	factory, err := registry.GetFactory("myfacade", 123)
	c.Assert(err, jc.ErrorIsNil)
	val, err := factory(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(val, gc.Equals, "myobject")
}

func (*RegistrySuite) TestGetFactoryUnknown(c *gc.C) {
	registry := &facade.Registry{}
	factory, err := registry.GetFactory("name", 0)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, `name\(0\) not found`)
	c.Check(factory, gc.IsNil)
}

func (*RegistrySuite) TestGetFactoryUnknownVersion(c *gc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)

	factory, err := registry.GetFactory("name", 1)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, `name\(1\) not found`)
	c.Check(factory, gc.IsNil)
}

func (*RegistrySuite) TestRegisterAndList(c *gc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)

	c.Check(registry.List(), jc.DeepEquals, []facade.Description{
		{Name: "name", Versions: []int{0}},
	})
}

func (*RegistrySuite) TestRegisterAndListSorted(c *gc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 10)
	assertRegister(c, registry, "name", 0)
	assertRegister(c, registry, "name", 101)

	c.Check(registry.List(), jc.DeepEquals, []facade.Description{
		{Name: "name", Versions: []int{0, 10, 101}},
	})
}

func (*RegistrySuite) TestRegisterAndListMultiple(c *gc.C) {
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

func (s *RegistrySuite) TestRegisterAndListMultipleWithFeatures(c *gc.C) {
	registry := &facade.Registry{}
	assertRegisterFlag(c, registry, "other", 0, "special")
	assertRegister(c, registry, "name", 0)
	assertRegisterFlag(c, registry, "name", 1, "special")
	assertRegister(c, registry, "third", 2)
	assertRegisterFlag(c, registry, "third", 3, "magic")

	s.SetFeatureFlags("magic")
	c.Check(registry.List(), jc.DeepEquals, []facade.Description{
		{Name: "name", Versions: []int{0}},
		{Name: "third", Versions: []int{2, 3}},
	})
}

func (*RegistrySuite) TestRegisterAlreadyPresent(c *gc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)
	secondIdFactory := func(context facade.Context) (facade.Facade, error) {
		var i = 200
		return &i, nil
	}
	err := registry.Register("name", 0, secondIdFactory, intPtrType, "")
	c.Assert(err, gc.ErrorMatches, `object "name\(0\)" already registered`)

	factory, err := registry.GetFactory("name", 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(factory, gc.NotNil)
	val, err := factory(nil)
	c.Assert(err, jc.ErrorIsNil)
	asIntPtr := val.(*int)
	c.Check(*asIntPtr, gc.Equals, 100)
}

func (*RegistrySuite) TestGetFactory(c *gc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)

	factory, err := registry.GetFactory("name", 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(factory, gc.NotNil)
	res, err := factory(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.NotNil)
	asIntPtr := res.(*int)
	c.Check(*asIntPtr, gc.Equals, 100)
}

func (*RegistrySuite) TestGetType(c *gc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)

	typ, err := registry.GetType("name", 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(typ, gc.Equals, intPtrType)
}

func (*RegistrySuite) TestDiscardHandlesNotPresent(c *gc.C) {
	registry := &facade.Registry{}
	registry.Discard("name", 1)
}

func (*RegistrySuite) TestDiscardRemovesEntry(c *gc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)
	_, err := registry.GetFactory("name", 0)
	c.Assert(err, jc.ErrorIsNil)

	registry.Discard("name", 0)
	factory, err := registry.GetFactory("name", 0)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, `name\(0\) not found`)
	c.Check(factory, gc.IsNil)
	c.Check(registry.List(), jc.DeepEquals, []facade.Description{})
}

func (*RegistrySuite) TestDiscardLeavesOtherVersions(c *gc.C) {
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

func testFacade(facade.Context) (facade.Facade, error) {
	return "myobject", nil
}

func validIdFactory(facade.Context) (facade.Facade, error) {
	var i = 100
	return &i, nil
}

var intPtr = new(int)
var intPtrType = reflect.TypeOf(&intPtr).Elem()

func assertRegister(c *gc.C, registry *facade.Registry, name string, version int) {
	assertRegisterFlag(c, registry, name, version, "")
}

func assertRegisterFlag(c *gc.C, registry *facade.Registry, name string, version int, flag string) {

	err := registry.Register(name, version, validIdFactory, intPtrType, flag)
	c.Assert(err, gc.IsNil)
}
