// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/core/facades"
	coretesting "github.com/juju/juju/internal/testing"
)

type facadeVersionSuite struct {
	coretesting.BaseSuite
}

func TestFacadeVersionSuite(t *testing.T) {
	tc.Run(t, &facadeVersionSuite{})
}

func (s *facadeVersionSuite) TestFacadeVersionsMatchServerVersions(c *tc.C) {
	// The client side code doesn't want to directly import the server side
	// code just to list out what versions are available. However, we do
	// want to make sure that the two sides are kept in sync.
	clientFacadeNames := set.NewStrings()
	for name, versions := range api.SupportedFacadeVersions() {
		clientFacadeNames.Add(name)
		// All versions should now be non-zero.
		c.Check(set.NewInts(versions...).Contains(0), tc.IsFalse)
	}
	allServerFacades := apiserver.AllFacades().List()
	serverFacadeNames := set.NewStrings()
	serverFacadeBestVersions := make(map[string]int, len(allServerFacades))
	for _, facade := range allServerFacades {
		serverFacadeNames.Add(facade.Name)
		serverFacadeBestVersions[facade.Name] = facade.Versions[len(facade.Versions)-1]
	}
	// First check that both sides know about all the same versions
	c.Check(serverFacadeNames.Difference(clientFacadeNames).SortedValues(), tc.HasLen, 0)
	c.Check(clientFacadeNames.Difference(serverFacadeNames).SortedValues(), tc.HasLen, 0)

	// Next check that the latest version of each facade is the same
	// on both sides.
	apiFacadeVersions := make(map[string]int)
	for name, versions := range api.SupportedFacadeVersions() {
		// Sort the versions so that we can easily pick the latest, without
		// a requirement that the versions are listed in order.
		sorted := set.NewInts(versions...).SortedValues()
		apiFacadeVersions[name] = sorted[len(sorted)-1]
	}
	c.Check(apiFacadeVersions, tc.DeepEquals, serverFacadeBestVersions)
}

// TestClientSupport checks that the client facade supports the 3.x and 4.x
// for certain tasks. You must be very careful when removing support for facades
// as it can break model migrations, upgrades, and state reports.
func (s *facadeVersionSuite) TestClientSupport(c *tc.C) {
	tests := []struct {
		facadeName       string
		summary          string
		apiClientVersion facades.FacadeVersion
	}{
		{
			facadeName:       "Client",
			summary:          "Ensure that the Client facade supports 3.6+ for status requests",
			apiClientVersion: []int{8},
		},
		{
			facadeName:       "ModelManager",
			summary:          "Ensure that the ModelManager facade supports 3.x for model migration and status requests",
			apiClientVersion: []int{9},
		},
	}
	for _, test := range tests {
		c.Logf("%s", test.summary)
		c.Check(api.SupportedFacadeVersions()[test.facadeName], Contains, test.apiClientVersion)
	}
}

type containsChecker struct {
	*tc.CheckerInfo
}

// Contains checks that the obtained slice contains the expected value.
var Contains tc.Checker = &containsChecker{
	CheckerInfo: &tc.CheckerInfo{Name: "Contains", Params: []string{"obtained", "expected"}},
}

func (checker *containsChecker) Check(params []interface{}, names []string) (result bool, err string) {
	expected, ok := params[1].(facades.FacadeVersion)
	if !ok {
		return false, "expected must be a int"
	}

	obtained, ok := params[0].(facades.FacadeVersion)
	if ok {
		if set.NewInts(expected...).Intersection(set.NewInts(obtained...)).Size() > 0 {
			return true, ""
		}

		return false, ""
	}

	return false, "obtained value is not an []int"
}
