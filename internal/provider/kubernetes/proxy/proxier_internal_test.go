// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/caas/kubernetes"
)

func TestProxierBroken(c *testing.T) {
	tc.Run(c, func(c *tc.C) {
		p := &Proxier{}
		c.Check(p.Broken() == nil, tc.IsTrue)

		p.tunnel = kubernetes.NewTunnel(nil, nil, kubernetes.TunnelKindPods, "", "", "")
		c.Check(p.Broken(), tc.Equals, p.tunnel.Broken())
	})
}
