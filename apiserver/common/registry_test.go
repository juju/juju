// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"reflect"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type facadeRegistrySuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&facadeRegistrySuite{})

func testFacade(
	st *state.State, resources *common.Resources,
	authorizer common.Authorizer, id string,
) (interface{}, error) {
	return "myobject", nil
}

func (s *facadeRegistrySuite) TestRegister(c *gc.C) {
	common.SanitizeFacades(s)
	var v interface{}
	common.RegisterFacade("myfacade", 0, testFacade, reflect.TypeOf(&v).Elem())
	f, err := common.Facades.GetFactory("myfacade", 0)
	c.Assert(err, jc.ErrorIsNil)
	val, err := f(nil, nil, nil, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(val, gc.Equals, "myobject")
}

func (*facadeRegistrySuite) TestGetFactoryUnknown(c *gc.C) {
	r := &common.FacadeRegistry{}
	f, err := r.GetFactory("name", 0)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, `name\(0\) not found`)
	c.Check(f, gc.IsNil)
}

func (s *facadeRegistrySuite) TestRegisterForFeature(c *gc.C) {
	common.SanitizeFacades(s)
	var v interface{}
	common.RegisterFacadeForFeature("myfacade", 0, testFacade, reflect.TypeOf(&v).Elem(), "magic")
	f, err := common.Facades.GetFactory("myfacade", 0)
	c.Check(err, jc.Satisfies, errors.IsNotFound)

	s.SetFeatureFlags("magic")

	f, err = common.Facades.GetFactory("myfacade", 0)
	c.Assert(err, jc.ErrorIsNil)
	val, err := f(nil, nil, nil, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(val, gc.Equals, "myobject")
}

func (*facadeRegistrySuite) TestGetFactoryUnknownVersion(c *gc.C) {
	r := &common.FacadeRegistry{}
	c.Assert(r.Register("name", 0, validIdFactory, intPtrType, ""), gc.IsNil)
	f, err := r.GetFactory("name", 1)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, `name\(1\) not found`)
	c.Check(f, gc.IsNil)
}

func (s *facadeRegistrySuite) TestRegisterFacadePanicsOnDoubleRegistry(c *gc.C) {
	var v interface{}
	doRegister := func() {
		common.RegisterFacade("myfacade", 0, testFacade, reflect.TypeOf(v))
	}
	doRegister()
	c.Assert(doRegister, gc.PanicMatches, `object "myfacade\(0\)" already registered`)
}

func checkValidateNewFacadeFailsWith(c *gc.C, obj interface{}, errMsg string) {
	err := common.ValidateNewFacade(reflect.ValueOf(obj))
	c.Check(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, errMsg)
}

func noArgs() {
}

func badCountIn(a string) (*int, error) {
	return nil, nil
}

func badCountOut(a, b, c string) error {
	return nil
}

func wrongIn(a, b, c string) (*int, error) {
	return nil, nil
}

func wrongOut(*state.State, *common.Resources, common.Authorizer) (error, *int) {
	return nil, nil
}

func validFactory(*state.State, *common.Resources, common.Authorizer) (*int, error) {
	var i = 100
	return &i, nil
}

func (*facadeRegistrySuite) TestValidateNewFacade(c *gc.C) {
	checkValidateNewFacadeFailsWith(c, nil,
		`cannot wrap nil`)
	checkValidateNewFacadeFailsWith(c, "notafunc",
		`wrong type "string" is not a function`)
	checkValidateNewFacadeFailsWith(c, noArgs,
		`function ".*noArgs" does not take 3 parameters and return 2`)
	checkValidateNewFacadeFailsWith(c, badCountIn,
		`function ".*badCountIn" does not take 3 parameters and return 2`)
	checkValidateNewFacadeFailsWith(c, badCountOut,
		`function ".*badCountOut" does not take 3 parameters and return 2`)
	checkValidateNewFacadeFailsWith(c, wrongIn,
		`function ".*wrongIn" does not have the signature func \(\*state.State, \*common.Resources, common.Authorizer\) \(\*Type, error\)`)
	checkValidateNewFacadeFailsWith(c, wrongOut,
		`function ".*wrongOut" does not have the signature func \(\*state.State, \*common.Resources, common.Authorizer\) \(\*Type, error\)`)
	err := common.ValidateNewFacade(reflect.ValueOf(validFactory))
	c.Assert(err, jc.ErrorIsNil)
}

func (*facadeRegistrySuite) TestWrapNewFacadeFailure(c *gc.C) {
	_, _, err := common.WrapNewFacade("notafunc")
	c.Check(err, gc.ErrorMatches, `wrong type "string" is not a function`)
}

func (*facadeRegistrySuite) TestWrapNewFacadeHandlesId(c *gc.C) {
	wrapped, _, err := common.WrapNewFacade(validFactory)
	c.Assert(err, jc.ErrorIsNil)
	val, err := wrapped(nil, nil, nil, "badId")
	c.Check(err, gc.ErrorMatches, "id not found")
	c.Check(val, gc.Equals, nil)
}

func (*facadeRegistrySuite) TestWrapNewFacadeCallsFunc(c *gc.C) {
	wrapped, _, err := common.WrapNewFacade(validFactory)
	c.Assert(err, jc.ErrorIsNil)
	val, err := wrapped(nil, nil, nil, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*(val.(*int)), gc.Equals, 100)
}

type myResult struct {
	st        *state.State
	resources *common.Resources
	auth      common.Authorizer
}

func (*facadeRegistrySuite) TestWrapNewFacadeCallsWithRightParams(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{}
	resources := common.NewResources()
	testFunc := func(
		st *state.State, resources *common.Resources,
		authorizer common.Authorizer,
	) (*myResult, error) {
		return &myResult{st, resources, authorizer}, nil
	}
	wrapped, facadeType, err := common.WrapNewFacade(testFunc)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(facadeType, gc.Equals, reflect.TypeOf((*myResult)(nil)))
	val, err := wrapped(nil, resources, authorizer, "")
	c.Assert(err, jc.ErrorIsNil)
	asResult := val.(*myResult)
	c.Check(asResult.st, gc.IsNil)
	c.Check(asResult.resources, gc.Equals, resources)
	c.Check(asResult.auth, gc.Equals, authorizer)
}

func (s *facadeRegistrySuite) TestRegisterStandardFacade(c *gc.C) {
	common.SanitizeFacades(s)
	common.RegisterStandardFacade("testing", 0, validFactory)
	wrapped, err := common.Facades.GetFactory("testing", 0)
	c.Assert(err, jc.ErrorIsNil)
	val, err := wrapped(nil, nil, nil, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*(val.(*int)), gc.Equals, 100)
}

func (s *facadeRegistrySuite) TestRegisterStandardFacadePanic(c *gc.C) {
	common.SanitizeFacades(s)
	c.Assert(
		func() { common.RegisterStandardFacade("badtest", 0, noArgs) },
		gc.PanicMatches,
		`function ".*noArgs" does not take 3 parameters and return 2`)
	_, err := common.Facades.GetFactory("badtest", 0)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `badtest\(0\) not found`)
}

func (*facadeRegistrySuite) TestDiscardedAPIMethods(c *gc.C) {
	allFacades := common.Facades.List()
	c.Assert(allFacades, gc.Not(gc.HasLen), 0)
	for _, description := range allFacades {
		for _, version := range description.Versions {
			facadeType, err := common.Facades.GetType(description.Name, version)
			c.Assert(err, jc.ErrorIsNil)
			facadeObjType := rpcreflect.ObjTypeOf(facadeType)
			// We must have some methods on every object returned
			// by a root-level method.
			c.Assert(facadeObjType.MethodNames(), gc.Not(gc.HasLen), 0)
			// We don't allow any methods that don't implement
			// an RPC entry point.
			c.Assert(facadeObjType.DiscardedMethods(), gc.HasLen, 0)
		}
	}
}

func validIdFactory(*state.State, *common.Resources, common.Authorizer, string) (interface{}, error) {
	var i = 100
	return &i, nil
}

var intPtr = new(int)
var intPtrType = reflect.TypeOf(&intPtr).Elem()

func (*facadeRegistrySuite) TestDescriptionFromVersions(c *gc.C) {
	facades := common.Versions{0: common.NilFacadeRecord}
	c.Check(common.DescriptionFromVersions("name", facades),
		gc.DeepEquals,
		common.FacadeDescription{
			Name:     "name",
			Versions: []int{0},
		})
	facades[2] = common.NilFacadeRecord
	c.Check(common.DescriptionFromVersions("name", facades),
		gc.DeepEquals,
		common.FacadeDescription{
			Name:     "name",
			Versions: []int{0, 2},
		})
}

func (*facadeRegistrySuite) TestDescriptionFromVersionsAreSorted(c *gc.C) {
	facades := common.Versions{
		10: common.NilFacadeRecord,
		5:  common.NilFacadeRecord,
		0:  common.NilFacadeRecord,
		18: common.NilFacadeRecord,
		6:  common.NilFacadeRecord,
		4:  common.NilFacadeRecord,
	}
	c.Check(common.DescriptionFromVersions("name", facades),
		gc.DeepEquals,
		common.FacadeDescription{
			Name:     "name",
			Versions: []int{0, 4, 5, 6, 10, 18},
		})
}

func (*facadeRegistrySuite) TestRegisterAndList(c *gc.C) {
	r := &common.FacadeRegistry{}
	c.Assert(r.Register("name", 0, validIdFactory, intPtrType, ""), gc.IsNil)
	c.Check(r.List(), gc.DeepEquals, []common.FacadeDescription{
		{Name: "name", Versions: []int{0}},
	})
}

func (*facadeRegistrySuite) TestRegisterAndListMultiple(c *gc.C) {
	r := &common.FacadeRegistry{}
	c.Assert(r.Register("other", 0, validIdFactory, intPtrType, ""), gc.IsNil)
	c.Assert(r.Register("name", 0, validIdFactory, intPtrType, ""), gc.IsNil)
	c.Assert(r.Register("third", 2, validIdFactory, intPtrType, ""), gc.IsNil)
	c.Assert(r.Register("third", 3, validIdFactory, intPtrType, ""), gc.IsNil)
	c.Check(r.List(), gc.DeepEquals, []common.FacadeDescription{
		{Name: "name", Versions: []int{0}},
		{Name: "other", Versions: []int{0}},
		{Name: "third", Versions: []int{2, 3}},
	})
}

func (s *facadeRegistrySuite) TestRegisterAndListMultipleWithFeatures(c *gc.C) {
	r := &common.FacadeRegistry{}
	c.Assert(r.Register("other", 0, validIdFactory, intPtrType, "special"), gc.IsNil)
	c.Assert(r.Register("name", 0, validIdFactory, intPtrType, ""), gc.IsNil)
	c.Assert(r.Register("name", 1, validIdFactory, intPtrType, "special"), gc.IsNil)
	c.Assert(r.Register("third", 2, validIdFactory, intPtrType, ""), gc.IsNil)
	c.Assert(r.Register("third", 3, validIdFactory, intPtrType, "magic"), gc.IsNil)
	s.SetFeatureFlags("magic")
	c.Check(r.List(), gc.DeepEquals, []common.FacadeDescription{
		{Name: "name", Versions: []int{0}},
		{Name: "third", Versions: []int{2, 3}},
	})
}

func (*facadeRegistrySuite) TestRegisterAlreadyPresent(c *gc.C) {
	r := &common.FacadeRegistry{}
	err := r.Register("name", 0, validIdFactory, intPtrType, "")
	c.Assert(err, jc.ErrorIsNil)
	secondIdFactory := func(*state.State, *common.Resources, common.Authorizer, string) (interface{}, error) {
		var i = 200
		return &i, nil
	}
	err = r.Register("name", 0, secondIdFactory, intPtrType, "")
	c.Check(err, gc.ErrorMatches, `object "name\(0\)" already registered`)
	f, err := r.GetFactory("name", 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f, gc.NotNil)
	val, err := f(nil, nil, nil, "")
	c.Assert(err, jc.ErrorIsNil)
	asIntPtr := val.(*int)
	c.Check(*asIntPtr, gc.Equals, 100)
}

func (*facadeRegistrySuite) TestGetFactory(c *gc.C) {
	r := &common.FacadeRegistry{}
	c.Assert(r.Register("name", 0, validIdFactory, intPtrType, ""), gc.IsNil)
	f, err := r.GetFactory("name", 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f, gc.NotNil)
	res, err := f(nil, nil, nil, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.NotNil)
	asIntPtr := res.(*int)
	c.Check(*asIntPtr, gc.Equals, 100)
}

func (*facadeRegistrySuite) TestGetType(c *gc.C) {
	r := &common.FacadeRegistry{}
	c.Assert(r.Register("name", 0, validIdFactory, intPtrType, ""), gc.IsNil)
	typ, err := r.GetType("name", 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(typ, gc.Equals, intPtrType)
}

func (*facadeRegistrySuite) TestDiscardHandlesNotPresent(c *gc.C) {
	r := &common.FacadeRegistry{}
	r.Discard("name", 1)
}

func (*facadeRegistrySuite) TestDiscardRemovesEntry(c *gc.C) {
	r := &common.FacadeRegistry{}
	c.Assert(r.Register("name", 0, validIdFactory, intPtrType, ""), gc.IsNil)
	_, err := r.GetFactory("name", 0)
	c.Assert(err, jc.ErrorIsNil)
	r.Discard("name", 0)
	f, err := r.GetFactory("name", 0)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, `name\(0\) not found`)
	c.Check(f, gc.IsNil)
	c.Check(r.List(), gc.DeepEquals, []common.FacadeDescription{})
}

func (*facadeRegistrySuite) TestDiscardLeavesOtherVersions(c *gc.C) {
	r := &common.FacadeRegistry{}
	c.Assert(r.Register("name", 0, validIdFactory, intPtrType, ""), gc.IsNil)
	c.Assert(r.Register("name", 1, validIdFactory, intPtrType, ""), gc.IsNil)
	r.Discard("name", 0)
	_, err := r.GetFactory("name", 0)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	_, err = r.GetFactory("name", 1)
	c.Check(err, jc.ErrorIsNil)
	c.Check(r.List(), gc.DeepEquals, []common.FacadeDescription{
		{Name: "name", Versions: []int{1}},
	})
}
