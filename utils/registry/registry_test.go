// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry_test

import (
	"reflect"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/errors"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/registry"
)

type registrySuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&registrySuite{})

type Factory func() (interface{}, error)

func nilFactory() (interface{}, error) {
	return nil, nil
}

var factoryType = reflect.TypeOf((*Factory)(nil)).Elem()

type testFacade struct {
	version string
	called  bool
}

type stringVal struct {
	value string
}

func (t *testFacade) TestMethod() stringVal {
	t.called = true
	return stringVal{"called " + t.version}
}

func (s *registrySuite) TestDescriptionFromVersions(c *gc.C) {
	versions := registry.Versions{0: nilFactory}
	c.Check(registry.DescriptionFromVersions("name", versions),
		gc.DeepEquals,
		registry.Description{
			Name:     "name",
			Versions: []int{0},
		})
	versions[2] = nilFactory
	c.Check(registry.DescriptionFromVersions("name", versions),
		gc.DeepEquals,
		registry.Description{
			Name:     "name",
			Versions: []int{0, 2},
		})
}

func (s *registrySuite) TestDescriptionFromVersionsAreSorted(c *gc.C) {
	versions := registry.Versions{
		10: nilFactory,
		5:  nilFactory,
		0:  nilFactory,
		18: nilFactory,
		6:  nilFactory,
		4:  nilFactory,
	}
	c.Check(registry.DescriptionFromVersions("name", versions),
		gc.DeepEquals,
		registry.Description{
			Name:     "name",
			Versions: []int{0, 4, 5, 6, 10, 18},
		})
}

func (s *registrySuite) TestRegisterAndList(c *gc.C) {
	r := registry.NewTypedNameVersion(factoryType)
	c.Assert(r.Register("name", 0, nilFactory), gc.IsNil)
	c.Check(r.List(), gc.DeepEquals, []registry.Description{
		{Name: "name", Versions: []int{0}},
	})
}

func (s *registrySuite) TestRegisterAndListMultiple(c *gc.C) {
	r := registry.NewTypedNameVersion(factoryType)
	c.Assert(r.Register("other", 0, nilFactory), gc.IsNil)
	c.Assert(r.Register("name", 0, nilFactory), gc.IsNil)
	c.Assert(r.Register("third", 2, nilFactory), gc.IsNil)
	c.Check(r.List(), gc.DeepEquals, []registry.Description{
		{Name: "name", Versions: []int{0}},
		{Name: "other", Versions: []int{0}},
		{Name: "third", Versions: []int{2}},
	})
}

func (s *registrySuite) TestRegisterWrongType(c *gc.C) {
	r := registry.NewTypedNameVersion(factoryType)
	err := r.Register("other", 0, "notAFactory")
	c.Check(err, gc.ErrorMatches, `object of type string cannot be converted to type .*registry_test.Factory`)
}

func (s *registrySuite) TestRegisterAlreadyPresent(c *gc.C) {
	r := registry.NewTypedNameVersion(factoryType)
	err := r.Register("name", 0, func() (interface{}, error) {
		return "orig", nil
	})
	c.Assert(err, gc.IsNil)
	err = r.Register("name", 0, func() (interface{}, error) {
		return "broken", nil
	})
	c.Check(err, gc.ErrorMatches, `object "name\(0\)" already registered`)
	f, err := r.Get("name", 0)
	c.Assert(err, gc.IsNil)
	val, err := f.(Factory)()
	c.Assert(err, gc.IsNil)
	c.Check(val, gc.Equals, "orig")
}

func (s *registrySuite) TestGet(c *gc.C) {
	r := registry.NewTypedNameVersion(factoryType)
	customFactory := func() (interface{}, error) {
		return 10, nil
	}
	c.Assert(r.Register("name", 0, customFactory), gc.IsNil)
	f, err := r.Get("name", 0)
	c.Assert(err, gc.IsNil)
	c.Assert(f, gc.NotNil)
	res, err := f.(Factory)()
	c.Assert(err, gc.IsNil)
	c.Check(res, gc.Equals, 10)
}

func (s *registrySuite) TestGetUnknown(c *gc.C) {
	r := registry.NewTypedNameVersion(factoryType)
	f, err := r.Get("name", 0)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, `name\(0\) not found`)
	c.Check(f, gc.IsNil)
}

func (s *registrySuite) TestGetUnknownVersion(c *gc.C) {
	r := registry.NewTypedNameVersion(factoryType)
	c.Assert(r.Register("name", 0, nilFactory), gc.IsNil)
	f, err := r.Get("name", 1)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, `name\(1\) not found`)
	c.Check(f, gc.IsNil)
}
