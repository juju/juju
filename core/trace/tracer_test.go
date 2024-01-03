// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"fmt"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/database"
)

type nameSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&nameSuite{})

func (nameSuite) TestNameFromFuncMethod(c *gc.C) {
	name := NameFromFunc()
	c.Assert(name, gc.Equals, Name("trace.nameSuite.TestNameFromFuncMethod"))
}

func (nameSuite) TestControllerNamespaceConstant(c *gc.C) {
	c.Assert(controllerNamespace, gc.Equals, database.ControllerNS)
}

type namespaceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&namespaceSuite{})

func (namespaceSuite) TestNamespaceShortNamespace(c *gc.C) {
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
		c.Assert(ns.ShortNamespace(), gc.Equals, test.expected)
	}
}

func (namespaceSuite) TestNamespaceString(c *gc.C) {
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
		c.Assert(ns.String(), gc.Equals, test.expected)
	}
}
