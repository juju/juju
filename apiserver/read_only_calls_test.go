// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type readOnlyCallsSuite struct {
}

var _ = gc.Suite(&readOnlyCallsSuite{})

func (*readOnlyCallsSuite) TestReadOnlyCall(c *gc.C) {
	for _, test := range []struct {
		facade string
		method string
	}{
		{"Action", "Actions"},
		{"Client", "FullStatus"},
		{"Service", "ServiceGet"},
		{"Storage", "List"},
	} {
		c.Logf("check %s.%s", test.facade, test.method)
		c.Check(isCallReadOnly(test.facade, test.method), jc.IsTrue)
	}
}

func (*readOnlyCallsSuite) TestWritableCalls(c *gc.C) {
	for _, test := range []struct {
		facade string
		method string
	}{
		{"Client", "UnknownMethod"},
		{"Service", "ServiceDeploy"},
		{"UnknownFacade", "List"},
	} {
		c.Logf("check %s.%s", test.facade, test.method)
		c.Check(isCallReadOnly(test.facade, test.method), jc.IsFalse)
	}
}
