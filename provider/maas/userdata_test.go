// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package maas_test

import (
	"encoding/base64"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/os"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/cloudinit/cloudinittest"
	"github.com/juju/juju/cloudconfig/providerinit/renderers"
	"github.com/juju/juju/provider/maas"
	"github.com/juju/juju/testing"
)

type RenderersSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&RenderersSuite{})

func (s *RenderersSuite) TestMAASUnix(c *gc.C) {
	renderer := maas.MAASRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{YAML: []byte("yaml")}

	result, err := renderer.Render(cloudcfg, os.Ubuntu)
	c.Assert(err, jc.ErrorIsNil)
	expected := base64.StdEncoding.EncodeToString(utils.Gzip(cloudcfg.YAML))
	c.Assert(string(result), jc.DeepEquals, expected)

	result, err = renderer.Render(cloudcfg, os.CentOS)
	c.Assert(err, jc.ErrorIsNil)
	expected = base64.StdEncoding.EncodeToString(utils.Gzip(cloudcfg.YAML))
	c.Assert(string(result), jc.DeepEquals, expected)
}

func (s *RenderersSuite) TestMAASWindows(c *gc.C) {
	renderer := maas.MAASRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{YAML: []byte("yaml")}

	result, err := renderer.Render(cloudcfg, os.Windows)
	c.Assert(err, jc.ErrorIsNil)
	expected := base64.StdEncoding.EncodeToString(renderers.WinEmbedInScript(cloudcfg.YAML))
	c.Assert(string(result), jc.DeepEquals, expected)
}

func (s *RenderersSuite) TestMAASUnknownOS(c *gc.C) {
	renderer := maas.MAASRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{}
	result, err := renderer.Render(cloudcfg, os.Arch)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "Cannot encode userdata for OS: Arch")
}
