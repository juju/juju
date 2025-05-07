// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"fmt"

	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/database"
)

type nameSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&nameSuite{})

func (nameSuite) TestNameFromFuncMethod(c *tc.C) {
	name := NameFromFunc()
	c.Assert(name, tc.Equals, Name("trace.nameSuite.TestNameFromFuncMethod"))
}

func (nameSuite) TestControllerNamespaceConstant(c *tc.C) {
	c.Assert(controllerNamespace, tc.Equals, database.ControllerNS)
}

type namespaceSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&namespaceSuite{})

func (namespaceSuite) TestNamespaceShortNamespace(c *tc.C) {
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

func (namespaceSuite) TestNamespaceString(c *tc.C) {
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
