// Copyright 2014-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade_test

import (
	"reflect"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type RegistrySuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&RegistrySuite{})

var (
	interfaceType = reflect.TypeOf((*interface{})(nil)).Elem()
	intPtrType    = reflect.TypeOf((*int)(nil))
)

func (s *RegistrySuite) TestRegister(c *gc.C) {
	registry := &facade.Registry{}
	err := registry.Register("myfacade", 123, testFacade, interfaceType)
	c.Assert(err, jc.ErrorIsNil)

	factory, err := registry.GetFactory("myfacade", 123)
	c.Assert(err, jc.ErrorIsNil)
	val, err := factory(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(val, gc.Equals, "myobject")
}

func (s *RegistrySuite) TestListDetails(c *gc.C) {
	registry := &facade.Registry{}
	err := registry.Register("f2", 6, testFacade, interfaceType)
	c.Assert(err, jc.ErrorIsNil)

	err = registry.Register("f1", 9, validIdFactory, intPtrType)
	c.Assert(err, jc.ErrorIsNil)

	details := registry.ListDetails()
	c.Assert(details, gc.HasLen, 2)
	c.Assert(details[0].Name, gc.Equals, "f1")
	c.Assert(details[0].Version, gc.Equals, 9)
	v, _ := details[0].Factory(nil)
	c.Assert(v, gc.FitsTypeOf, new(int))
	c.Assert(details[0].Type, gc.Equals, intPtrType)

	c.Assert(details[1].Name, gc.Equals, "f2")
	c.Assert(details[1].Version, gc.Equals, 6)
	v, _ = details[1].Factory(nil)
	c.Assert(v, gc.Equals, "myobject")
	c.Assert(details[1].Type, gc.Equals, interfaceType)
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

func (*RegistrySuite) TestRegisterAlreadyPresent(c *gc.C) {
	registry := &facade.Registry{}
	assertRegister(c, registry, "name", 0)
	secondIdFactory := func(context facade.Context) (facade.Facade, error) {
		var i = 200
		return &i, nil
	}
	err := registry.Register("name", 0, secondIdFactory, intPtrType)
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

func (*RegistrySuite) TestWrapNewFacadeFailure(c *gc.C) {
	_, _, err := facade.WrapNewFacade("notafunc")
	c.Check(err, gc.ErrorMatches, `wrong type "string" is not a function`)
}

func (*RegistrySuite) TestWrapNewFacadeHandlesId(c *gc.C) {
	wrapped, _, err := facade.WrapNewFacade(validFactory)
	c.Assert(err, jc.ErrorIsNil)
	val, err := wrapped(facadetest.Context{
		ID_: "badId",
	})
	c.Check(err, gc.ErrorMatches, "id not expected")
	c.Check(val, gc.Equals, nil)
}

func (*RegistrySuite) TestWrapNewFacadeCallsFunc(c *gc.C) {
	for _, function := range []interface{}{validFactory, validContextFactory} {
		wrapped, _, err := facade.WrapNewFacade(function)
		c.Assert(err, jc.ErrorIsNil)
		val, err := wrapped(facadetest.Context{})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(*(val.(*int)), gc.Equals, 100)
	}
}

func (*RegistrySuite) TestWrapNewFacadeCallsWithRightParams(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{}
	resources := common.NewResources()
	testFunc := func(
		st *state.State,
		resources facade.Resources,
		authorizer facade.Authorizer,
	) (*myResult, error) {
		return &myResult{st, resources, authorizer}, nil
	}
	wrapped, facadeType, err := facade.WrapNewFacade(testFunc)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(facadeType, gc.Equals, reflect.TypeOf((*myResult)(nil)))

	val, err := wrapped(facadetest.Context{
		Resources_: resources,
		Auth_:      authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)
	asResult := val.(*myResult)
	c.Check(asResult.st, gc.IsNil)
	c.Check(asResult.resources, gc.Equals, resources)
	c.Check(asResult.auth, gc.Equals, authorizer)
}

func (s *RegistrySuite) TestRegisterStandard(c *gc.C) {
	registry := &facade.Registry{}
	registry.RegisterStandard("testing", 0, validFactory)
	wrapped, err := registry.GetFactory("testing", 0)
	c.Assert(err, jc.ErrorIsNil)
	val, err := wrapped(facadetest.Context{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*(val.(*int)), gc.Equals, 100)
}

func (s *RegistrySuite) TestRegisterStandardError(c *gc.C) {
	registry := &facade.Registry{}
	err := registry.RegisterStandard("badtest", 0, noArgs)
	c.Assert(err, gc.ErrorMatches,
		`function ".*noArgs" does not have the signature .* or .*`)

	_, err = registry.GetFactory("badtest", 0)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `badtest\(0\) not found`)
}

func assertRegister(c *gc.C, registry *facade.Registry, name string, version int) {
	assertRegisterFlag(c, registry, name, version)
}

func assertRegisterFlag(c *gc.C, registry *facade.Registry, name string, version int) {
	err := registry.Register(name, version, validIdFactory, intPtrType)
	c.Assert(err, gc.IsNil)
}

func testFacade(_ facade.Context) (facade.Facade, error) {
	return "myobject", nil
}

func validIdFactory(_ facade.Context) (facade.Facade, error) {
	var i = 100
	return &i, nil
}

type myResult struct {
	st        *state.State
	resources facade.Resources
	auth      facade.Authorizer
}

func noArgs() {
}

func validFactory(_ *state.State, _ facade.Resources, _ facade.Authorizer) (*int, error) {
	var i = 100
	return &i, nil
}

func validContextFactory(_ facade.Context) (*int, error) {
	var i = 100
	return &i, nil
}
