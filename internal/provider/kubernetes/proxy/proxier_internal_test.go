// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/caas/kubernetes"
)

type proxierInternalSuite struct{}

func TestProxierInternalSuite(t *testing.T) {
	tc.Run(t, &proxierInternalSuite{})
}

func (s *proxierInternalSuite) TestBrokenDelegatesToTunnel(c *tc.C) {
	tunnel := kubernetes.NewTunnel(nil, nil, kubernetes.TunnelKindPods, "", "", "")
	proxier := &Proxier{tunnel: tunnel}

	c.Check(proxier.Broken(), tc.Equals, tunnel.Broken())
}
