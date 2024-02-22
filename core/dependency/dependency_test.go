// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	gc "gopkg.in/check.v1"
)

type dependencySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&dependencySuite{})

func (s *dependencySuite) TestGetDependencyByName(c *gc.C) {
	getter := dependencytesting.StubGetter(map[string]any{
		"foo": foo{},
	})

	result, err := GetDependencyByName[foo, bar](getter, "foo", func(foo foo) (bar, error) {
		return foo.Bar(), nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.FitsTypeOf, bar{})
}

func (s *dependencySuite) TestGetDependencyByNameNotFound(c *gc.C) {
	getter := dependencytesting.StubGetter(map[string]any{
		"foo": foo{},
	})

	_, err := GetDependencyByName[foo, bar](getter, "inferi", func(foo foo) (bar, error) {
		c.Fatalf("should not be called")
		return bar{}, nil
	})
	c.Assert(err, gc.ErrorMatches, `unexpected resource name: inferi`)
}

func (s *dependencySuite) TestGetDependencyByNameWithIdentity(c *gc.C) {
	getter := dependencytesting.StubGetter(map[string]any{
		"foo": foo{},
	})

	result, err := GetDependencyByName[foo, foo](getter, "foo", Identity[foo, foo])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.FitsTypeOf, foo{})
}

type foo struct{}

func (f foo) Bar() bar {
	return bar{}
}

type bar struct{}
