// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/testhelpers"
)

type nameSuite struct {
	testhelpers.IsolationSuite
}

func TestNameSuite(t *testing.T) {
	tc.Run(t, &nameSuite{})
}

func (s *nameSuite) TestNameFromFuncMethod(c *tc.C) {
	name := NameFromFunc()
	c.Assert(name, tc.Equals, Name("trace.(*nameSuite).TestNameFromFuncMethod"))
}

func (s *nameSuite) TestControllerNamespaceConstant(c *tc.C) {
	c.Assert(controllerNamespace, tc.Equals, database.ControllerNS)
}

type namespaceSuite struct {
	testhelpers.IsolationSuite
}

func TestNamespaceSuite(t *testing.T) {
	tc.Run(t, &namespaceSuite{})
}

func (s *namespaceSuite) TestNamespaceShortNamespace(c *tc.C) {
	tests := []struct {
		workerName string
		namespace  string
		expected   string
	}{{
		workerName: "foo",
		namespace:  "bar",
		expected:   "bar",
	}, {
		workerName: "foo",
		namespace:  "",
		expected:   "",
	}, {
		workerName: "foo",
		namespace:  "deadbeef",
		expected:   "deadbe",
	}, {
		workerName: "foo",
		namespace:  controllerNamespace,
		expected:   controllerNamespace,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.workerName)

		ns := Namespace(test.workerName, test.namespace)
		c.Assert(ns.ShortNamespace(), tc.Equals, test.expected)
	}
}

func (s *namespaceSuite) TestNamespaceString(c *tc.C) {
	tests := []struct {
		workerName string
		namespace  string
		expected   string
	}{{
		workerName: "foo",
		namespace:  "bar",
		expected:   "foo:bar",
	}, {
		workerName: "foo",
		namespace:  "",
		expected:   "foo",
	}, {
		workerName: "foo",
		namespace:  "deadbeef",
		expected:   "foo:deadbeef",
	}, {
		workerName: "foo",
		namespace:  controllerNamespace,
		expected:   fmt.Sprintf("foo:%s", controllerNamespace),
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.workerName)

		ns := Namespace(test.workerName, test.namespace)
		c.Assert(ns.String(), tc.Equals, test.expected)
	}
}
