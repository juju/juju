// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"strings"

	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/feature"
	coretesting "github.com/juju/juju/testing"
)

type facadeVersionSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&facadeVersionSuite{})

func (s *facadeVersionSuite) TestFacadeVersionsMatchServerVersions(c *gc.C) {
	// Enable feature flags so we can see them all.
	devFeatures := []string{feature.JES}
	s.SetFeatureFlags(strings.Join(devFeatures, ","))
	// The client side code doesn't want to directly import the server side
	// code just to list out what versions are available. However, we do
	// want to make sure that the two sides are kept in sync.
	clientFacadeNames := set.NewStrings()
	for name := range *api.FacadeVersions {
		clientFacadeNames.Add(name)
	}
	allServerFacades := common.Facades.List()
	serverFacadeNames := set.NewStrings()
	serverFacadeBestVersions := make(map[string]int, len(allServerFacades))
	for _, facade := range allServerFacades {
		serverFacadeNames.Add(facade.Name)
		serverFacadeBestVersions[facade.Name] = facade.Versions[len(facade.Versions)-1]
	}
	// First check that both sides know about all the same versions
	c.Check(serverFacadeNames.Difference(clientFacadeNames).SortedValues(), gc.HasLen, 0)
	c.Check(clientFacadeNames.Difference(serverFacadeNames).SortedValues(), gc.HasLen, 0)
	// Next check that the best versions match
	c.Check(*api.FacadeVersions, gc.DeepEquals, serverFacadeBestVersions)
}

func checkBestVersion(c *gc.C, desiredVersion int, versions []int, expectedVersion int) {
	resultVersion := api.BestVersion(desiredVersion, versions)
	c.Check(resultVersion, gc.Equals, expectedVersion)
}

func (*facadeVersionSuite) TestBestVersionDesiredAvailable(c *gc.C) {
	checkBestVersion(c, 0, []int{0, 1, 2}, 0)
	checkBestVersion(c, 1, []int{0, 1, 2}, 1)
	checkBestVersion(c, 2, []int{0, 1, 2}, 2)
}

func (*facadeVersionSuite) TestBestVersionDesiredNewer(c *gc.C) {
	checkBestVersion(c, 3, []int{0}, 0)
	checkBestVersion(c, 3, []int{0, 1, 2}, 2)
}

func (*facadeVersionSuite) TestBestVersionDesiredGap(c *gc.C) {
	checkBestVersion(c, 1, []int{0, 2}, 0)
}

func (*facadeVersionSuite) TestBestVersionNoVersions(c *gc.C) {
	checkBestVersion(c, 0, []int{}, 0)
	checkBestVersion(c, 1, []int{}, 0)
	checkBestVersion(c, 0, []int(nil), 0)
	checkBestVersion(c, 1, []int(nil), 0)
}

func (*facadeVersionSuite) TestBestVersionNotSorted(c *gc.C) {
	checkBestVersion(c, 0, []int{0, 3, 1, 2}, 0)
	checkBestVersion(c, 3, []int{0, 3, 1, 2}, 3)
	checkBestVersion(c, 1, []int{0, 3, 1, 2}, 1)
	checkBestVersion(c, 2, []int{0, 3, 1, 2}, 2)
}

func (s *facadeVersionSuite) TestBestFacadeVersionExactMatch(c *gc.C) {
	s.PatchValue(api.FacadeVersions, map[string]int{"Client": 1})
	st := api.NewTestingState(api.TestingStateParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1},
		}})
	c.Check(st.BestFacadeVersion("Client"), gc.Equals, 1)
}

func (s *facadeVersionSuite) TestBestFacadeVersionNewerServer(c *gc.C) {
	s.PatchValue(api.FacadeVersions, map[string]int{"Client": 1})
	st := api.NewTestingState(api.TestingStateParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1, 2},
		}})
	c.Check(st.BestFacadeVersion("Client"), gc.Equals, 1)
}

func (s *facadeVersionSuite) TestBestFacadeVersionNewerClient(c *gc.C) {
	s.PatchValue(api.FacadeVersions, map[string]int{"Client": 2})
	st := api.NewTestingState(api.TestingStateParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1},
		}})
	c.Check(st.BestFacadeVersion("Client"), gc.Equals, 1)
}

func (s *facadeVersionSuite) TestBestFacadeVersionServerUnknown(c *gc.C) {
	s.PatchValue(api.FacadeVersions, map[string]int{"TestingAPI": 2})
	st := api.NewTestingState(api.TestingStateParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1},
		}})
	c.Check(st.BestFacadeVersion("TestingAPI"), gc.Equals, 0)
}
