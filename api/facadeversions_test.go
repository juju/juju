// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/facades"
	coretesting "github.com/juju/juju/internal/testing"
)

type facadeVersionSuite struct {
	coretesting.BaseSuite
}

func TestFacadeVersionSuite(t *testing.T) {
	tc.Run(t, &facadeVersionSuite{})
}

func checkBestVersion(c *tc.C, desiredVersion, versions []int, expectedVersion int) {
	resultVersion := facades.BestVersion(desiredVersion, versions)
	c.Check(resultVersion, tc.Equals, expectedVersion)
}

func (*facadeVersionSuite) TestBestVersionDesiredAvailable(c *tc.C) {
	checkBestVersion(c, []int{0}, []int{0, 1, 2}, 0)
	checkBestVersion(c, []int{0, 1}, []int{0, 1, 2}, 1)
	checkBestVersion(c, []int{0, 1, 2}, []int{0, 1, 2}, 2)
}

func (*facadeVersionSuite) TestBestVersionDesiredNewer(c *tc.C) {
	checkBestVersion(c, []int{1, 2, 3}, []int{0}, 0)
	checkBestVersion(c, []int{1, 2, 3}, []int{0, 1, 2}, 2)
}

func (*facadeVersionSuite) TestBestVersionDesiredGap(c *tc.C) {
	checkBestVersion(c, []int{1}, []int{0, 2}, 0)
}

func (*facadeVersionSuite) TestBestVersionNoVersions(c *tc.C) {
	checkBestVersion(c, []int{0}, []int{}, 0)
	checkBestVersion(c, []int{1}, []int{}, 0)
	checkBestVersion(c, []int{0}, []int(nil), 0)
	checkBestVersion(c, []int{1}, []int(nil), 0)
}

func (*facadeVersionSuite) TestBestVersionNotSorted(c *tc.C) {
	checkBestVersion(c, []int{0}, []int{0, 3, 1, 2}, 0)
	checkBestVersion(c, []int{3}, []int{0, 3, 1, 2}, 3)
	checkBestVersion(c, []int{1}, []int{0, 3, 1, 2}, 1)
	checkBestVersion(c, []int{2}, []int{0, 3, 1, 2}, 2)
}

func (s *facadeVersionSuite) TestBestFacadeVersionExactMatch(c *tc.C) {
	s.PatchValue(api.FacadeVersions, map[string]facades.FacadeVersion{"Client": {1}})
	conn := api.NewTestingConnection(c, api.TestingConnectionParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1},
		}})
	c.Check(conn.BestFacadeVersion("Client"), tc.Equals, 1)
}

func (s *facadeVersionSuite) TestBestFacadeVersionNewerServer(c *tc.C) {
	s.PatchValue(api.FacadeVersions, map[string]facades.FacadeVersion{"Client": {1}})
	conn := api.NewTestingConnection(c, api.TestingConnectionParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1, 2},
		}})
	c.Check(conn.BestFacadeVersion("Client"), tc.Equals, 1)
}

func (s *facadeVersionSuite) TestBestFacadeVersionNewerClient(c *tc.C) {
	s.PatchValue(api.FacadeVersions, map[string]facades.FacadeVersion{"Client": {1, 2}})
	conn := api.NewTestingConnection(c, api.TestingConnectionParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1},
		}})
	c.Check(conn.BestFacadeVersion("Client"), tc.Equals, 1)
}

func (s *facadeVersionSuite) TestBestFacadeVersionServerUnknown(c *tc.C) {
	s.PatchValue(api.FacadeVersions, map[string]facades.FacadeVersion{"TestingAPI": {1, 2}})
	conn := api.NewTestingConnection(c, api.TestingConnectionParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1},
		}})
	c.Check(conn.BestFacadeVersion("TestingAPI"), tc.Equals, 0)
}
