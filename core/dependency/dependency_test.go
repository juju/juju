// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	stdtesting "testing"

	"github.com/juju/tc"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/internal/testhelpers"
)

type dependencySuite struct {
	testhelpers.IsolationSuite
}

func TestDependencySuite(t *stdtesting.T) { tc.Run(t, &dependencySuite{}) }
func (s *dependencySuite) TestGetDependencyByName(c *tc.C) {
	getter := dependencytesting.StubGetter(map[string]any{
		"foo": foo{},
	})

	result, err := GetDependencyByName[foo, bar](getter, "foo", func(foo foo) bar {
		return foo.Bar()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.FitsTypeOf, bar{})
}

func (s *dependencySuite) TestGetDependencyByNameNotFound(c *tc.C) {
	getter := dependencytesting.StubGetter(map[string]any{
		"foo": foo{},
	})

	_, err := GetDependencyByName[foo, bar](getter, "inferi", func(foo foo) bar {
		c.Fatalf("should not be called")
		return bar{}
	})
	c.Assert(err, tc.ErrorMatches, `unexpected resource name: inferi`)
}

func (s *dependencySuite) TestGetDependencyByNameWithIdentity(c *tc.C) {
	getter := dependencytesting.StubGetter(map[string]any{
		"foo": foo{},
	})

	result, err := GetDependencyByName[foo, foo](getter, "foo", Identity[foo, foo])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.FitsTypeOf, foo{})
}

type foo struct{}

func (f foo) Bar() bar {
	return bar{}
}

type bar struct{}
