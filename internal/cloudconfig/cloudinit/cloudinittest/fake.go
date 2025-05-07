// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinittest

import (
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/testhelpers"
)

type CloudConfig struct {
	cloudinit.CloudConfig
	testhelpers.Stub

	YAML   []byte
	Script string
}

func (c *CloudConfig) RenderYAML() ([]byte, error) {
	c.MethodCall(c, "RenderYAML")
	return c.YAML, c.NextErr()
}

func (c *CloudConfig) RenderScript() (string, error) {
	c.MethodCall(c, "RenderScript")
	return c.Script, c.NextErr()
}
