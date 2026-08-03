// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

type FQDNSuite struct{}

func TestFQDNSuite(t *testing.T) {
	tc.Run(t, &FQDNSuite{})
}

func (s *FQDNSuite) TestControllerPodFQDN(c *tc.C) {
	c.Check(
		utils.ControllerPodFQDN("controller-0", "controller-foo"),
		tc.Equals,
		"controller-0.controller-service-endpoints.controller-foo.svc.cluster.local",
	)
	c.Check(
		utils.ControllerPodFQDN("controller-2", "controller-bar"),
		tc.Equals,
		"controller-2.controller-service-endpoints.controller-bar.svc.cluster.local",
	)
}
