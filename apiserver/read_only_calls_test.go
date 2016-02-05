// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
)

type readOnlyCallsSuite struct {
}

var _ = gc.Suite(&readOnlyCallsSuite{})

func (*readOnlyCallsSuite) TestReadOnlyCallsExist(c *gc.C) {
	// Iterate through the list of readOnlyCalls and make sure
	// that the facades are reachable.
	facades := common.Facades.List()

	maxVersion := map[string]int{}
	for _, facade := range facades {
		version := 0
		for _, ver := range facade.Versions {
			if ver > version {
				version = ver
			}
		}
		maxVersion[facade.Name] = version
	}

	for _, name := range readOnlyCalls.Values() {
		parts := strings.Split(name, ".")
		facade, method := parts[0], parts[1]
		version := maxVersion[facade]

		_, _, err := lookupMethod(facade, version, method)
		c.Check(err, jc.ErrorIsNil)
	}
}

func (*readOnlyCallsSuite) TestReadOnlyCall(c *gc.C) {
	for _, test := range []struct {
		facade string
		method string
	}{
		{"Action", "Actions"},
		{"Client", "FullStatus"},
		{"Service", "Get"},
		{"Storage", "ListStorageDetails"},
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
		{"Service", "Deploy"},
		{"UnknownFacade", "List"},
	} {
		c.Logf("check %s.%s", test.facade, test.method)
		c.Check(isCallReadOnly(test.facade, test.method), jc.IsFalse)
	}
}
