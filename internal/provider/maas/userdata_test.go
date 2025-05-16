// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package maas_test

import (
	"encoding/base64"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/internal/cloudconfig/cloudinit/cloudinittest"
	"github.com/juju/juju/internal/provider/maas"
	"github.com/juju/juju/internal/testing"
)

type RenderersSuite struct {
	testing.BaseSuite
}

func TestRenderersSuite(t *stdtesting.T) { tc.Run(t, &RenderersSuite{}) }
func (s *RenderersSuite) TestMAASUnix(c *tc.C) {
	renderer := maas.MAASRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{YAML: []byte("yaml")}

	result, err := renderer.Render(cloudcfg, ostype.Ubuntu)
	c.Assert(err, tc.ErrorIsNil)
	expected := base64.StdEncoding.EncodeToString(utils.Gzip(cloudcfg.YAML))
	c.Assert(string(result), tc.DeepEquals, expected)
}

func (s *RenderersSuite) TestMAASUnknownOS(c *tc.C) {
	renderer := maas.MAASRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{}
	result, err := renderer.Render(cloudcfg, ostype.GenericLinux)
	c.Assert(result, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "Cannot encode userdata for OS: GenericLinux")
}
