// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/apiserver/common"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/utils/set"
)

type facadeVersionSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&facadeVersionSuite{})

func (*facadeVersionSuite) TestFacadeVersionsMatchServerVersions(c *gc.C) {
	// The client side code doesn't want to directly import the server side
	// code just to list out what versions are available. However, we do
	// want to make sure that the two sides are kept in sync.
	clientFacadeNames := set.NewStrings()
	for name, _ := range api.FacadeVersions {
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
	c.Check(serverFacadeNames.Difference(clientFacadeNames).SortedValues(), gc.DeepEquals, []string{})
	c.Check(clientFacadeNames.Difference(serverFacadeNames).SortedValues(), gc.DeepEquals, []string{})
	// Next check that the best versions match
	c.Check(api.FacadeVersions, gc.DeepEquals, serverFacadeBestVersions)
}
