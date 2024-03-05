// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/facades"
	coretesting "github.com/juju/juju/testing"
)

type facadeVersionSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&facadeVersionSuite{})

func checkBestVersion(c *gc.C, desiredVersion, versions []int, expectedVersion int) {
	resultVersion := facades.BestVersion(desiredVersion, versions)
	c.Check(resultVersion, gc.Equals, expectedVersion)
}

func (*facadeVersionSuite) TestBestVersionDesiredAvailable(c *gc.C) {
	checkBestVersion(c, []int{0}, []int{0, 1, 2}, 0)
	checkBestVersion(c, []int{0, 1}, []int{0, 1, 2}, 1)
	checkBestVersion(c, []int{0, 1, 2}, []int{0, 1, 2}, 2)
}

func (*facadeVersionSuite) TestBestVersionDesiredNewer(c *gc.C) {
	checkBestVersion(c, []int{1, 2, 3}, []int{0}, 0)
	checkBestVersion(c, []int{1, 2, 3}, []int{0, 1, 2}, 2)
}

func (*facadeVersionSuite) TestBestVersionDesiredGap(c *gc.C) {
	checkBestVersion(c, []int{1}, []int{0, 2}, 0)
}

func (*facadeVersionSuite) TestBestVersionNoVersions(c *gc.C) {
	checkBestVersion(c, []int{0}, []int{}, 0)
	checkBestVersion(c, []int{1}, []int{}, 0)
	checkBestVersion(c, []int{0}, []int(nil), 0)
	checkBestVersion(c, []int{1}, []int(nil), 0)
}

func (*facadeVersionSuite) TestBestVersionNotSorted(c *gc.C) {
	checkBestVersion(c, []int{0}, []int{0, 3, 1, 2}, 0)
	checkBestVersion(c, []int{3}, []int{0, 3, 1, 2}, 3)
	checkBestVersion(c, []int{1}, []int{0, 3, 1, 2}, 1)
	checkBestVersion(c, []int{2}, []int{0, 3, 1, 2}, 2)
}

func (s *facadeVersionSuite) TestBestFacadeVersionExactMatch(c *gc.C) {
	s.PatchValue(api.FacadeVersions, map[string]facades.FacadeVersion{"Client": {1}})
	conn := api.NewTestingConnection(api.TestingConnectionParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1},
		}})
	c.Check(conn.BestFacadeVersion("Client"), gc.Equals, 1)
}

func (s *facadeVersionSuite) TestBestFacadeVersionNewerServer(c *gc.C) {
	s.PatchValue(api.FacadeVersions, map[string]facades.FacadeVersion{"Client": {1}})
	conn := api.NewTestingConnection(api.TestingConnectionParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1, 2},
		}})
	c.Check(conn.BestFacadeVersion("Client"), gc.Equals, 1)
}

func (s *facadeVersionSuite) TestBestFacadeVersionNewerClient(c *gc.C) {
	s.PatchValue(api.FacadeVersions, map[string]facades.FacadeVersion{"Client": {1, 2}})
	conn := api.NewTestingConnection(api.TestingConnectionParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1},
		}})
	c.Check(conn.BestFacadeVersion("Client"), gc.Equals, 1)
}

func (s *facadeVersionSuite) TestBestFacadeVersionServerUnknown(c *gc.C) {
	s.PatchValue(api.FacadeVersions, map[string]facades.FacadeVersion{"TestingAPI": {1, 2}})
	conn := api.NewTestingConnection(api.TestingConnectionParams{
		FacadeVersions: map[string][]int{
			"Client": {0, 1},
		}})
	c.Check(conn.BestFacadeVersion("TestingAPI"), gc.Equals, 0)
}
