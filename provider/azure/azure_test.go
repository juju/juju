// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdtesting "testing"

	"github.com/juju/utils/set"
	gc "launchpad.net/gocheck"
	"launchpad.net/gwacl"

	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/testing"
)

func TestAzureProvider(t *stdtesting.T) {
	gc.TestingT(t)
}

type providerSuite struct {
	testing.BaseSuite
	envtesting.ToolsFixture
	restoreTimeouts func()
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.restoreTimeouts = envtesting.PatchAttemptStrategies()
}

func (s *providerSuite) TearDownSuite(c *gc.C) {
	s.restoreTimeouts()
	s.BaseSuite.TearDownSuite(c)
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(&getVirtualNetwork, func(*azureEnviron) (*gwacl.VirtualNetworkSite, error) {
		return &gwacl.VirtualNetworkSite{Name: "vnet", Location: "West US"}, nil
	})

	available := make(set.Strings)
	for _, rs := range gwacl.RoleSizes {
		available.Add(rs.Name)
	}
	s.PatchValue(&getAvailableRoleSizes, func(*azureEnviron) (set.Strings, error) {
		return available, nil
	})
}

func (s *providerSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}
