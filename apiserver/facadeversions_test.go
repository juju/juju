// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	coretesting "github.com/juju/juju/testing"
)

type facadeVersionSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&facadeVersionSuite{})

func (s *facadeVersionSuite) TestFacadeVersionsMatchServerVersions(c *gc.C) {
	// The client side code doesn't want to directly import the server side
	// code just to list out what versions are available. However, we do
	// want to make sure that the two sides are kept in sync.
	clientFacadeNames := set.NewStrings()
	for name, versions := range api.SupportedFacadeVersions() {
		clientFacadeNames.Add(name)
		// All versions should now be non-zero.
		c.Check(set.NewInts(versions...).Contains(0), jc.IsFalse)
	}
	allServerFacades := apiserver.AllFacades().List()
	serverFacadeNames := set.NewStrings()
	serverFacadeBestVersions := make(map[string]int, len(allServerFacades))
	for _, facade := range allServerFacades {
		serverFacadeNames.Add(facade.Name)
		serverFacadeBestVersions[facade.Name] = facade.Versions[len(facade.Versions)-1]
	}
	// First check that both sides know about all the same versions
	c.Check(serverFacadeNames.Difference(clientFacadeNames).SortedValues(), gc.HasLen, 0)
	c.Check(clientFacadeNames.Difference(serverFacadeNames).SortedValues(), gc.HasLen, 0)

	// Next check that the latest version of each facade is the same
	// on both sides.
	apiFacadeVersions := make(map[string]int)
	for name, versions := range api.SupportedFacadeVersions() {
		// Sort the versions so that we can easily pick the latest, without
		// a requirement that the versions are listed in order.
		sorted := set.NewInts(versions...).SortedValues()
		apiFacadeVersions[name] = sorted[len(sorted)-1]
	}
	c.Check(apiFacadeVersions, jc.DeepEquals, serverFacadeBestVersions)
}

// TestClient3xSupport checks that the client facade supports the 3.x for
// certain tasks. You must be very careful when removing support for facades
// as it can break model migrations, upgrades, and state reports.
func (s *facadeVersionSuite) TestClient3xSupport(c *gc.C) {
	tests := []struct {
		facadeName       string
		summary          string
		apiClientVersion int
	}{
		{
			facadeName:       "Client",
			summary:          "Ensure that the Client facade supports 3.x for status requests",
			apiClientVersion: 6,
		},
		{
			facadeName:       "ModelManager",
			summary:          "Ensure that the ModelManager facade supports 3.x for model migration and status requests",
			apiClientVersion: 9,
		},
	}
	for _, test := range tests {
		c.Logf(test.summary)
		c.Check(api.SupportedFacadeVersions()[test.facadeName], Contains, test.apiClientVersion)
	}
}

type containsChecker struct {
	*gc.CheckerInfo
}

// Contains checks that the obtained slice contains the expected value.
var Contains gc.Checker = &containsChecker{
	CheckerInfo: &gc.CheckerInfo{Name: "Contains", Params: []string{"obtained", "expected"}},
}

func (checker *containsChecker) Check(params []interface{}, names []string) (result bool, err string) {
	expected, ok := params[1].(int)
	if !ok {
		return false, "expected must be a string"
	}

	obtained, ok := params[0].([]int)
	if ok {
		for _, v := range obtained {
			if v == expected {
				return true, ""
			}
		}
		return false, ""
	}

	return false, "obtained value is not an []int"
}
