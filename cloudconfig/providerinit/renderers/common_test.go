// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package renderers_test

import (
	"encoding/base64"
	"fmt"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit/cloudinittest"
	"github.com/juju/juju/cloudconfig/providerinit/renderers"
	"github.com/juju/juju/testing"
)

type RenderersSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&RenderersSuite{})

func (s *RenderersSuite) TestToBase64(c *gc.C) {
	in := []byte("test")
	expected := base64.StdEncoding.EncodeToString(in)
	out := renderers.ToBase64(in)
	c.Assert(string(out), gc.Equals, expected)
}

func (s *RenderersSuite) TestWinEmbedInScript(c *gc.C) {
	in := []byte("test")
	encUserdata := renderers.ToBase64(utils.Gzip(in))
	var expected []byte
	header := "#ps1_sysnative\r\n"
	script := fmt.Sprintf(cloudconfig.UserDataScript, encUserdata)
	expected = append(expected, header...)
	expected = append(expected, script...)
	out := renderers.WinEmbedInScript(in)
	c.Assert(out, jc.DeepEquals, expected)
}

func (s *RenderersSuite) TestPrependWinPS1Header(c *gc.C) {
	in := []byte("test")
	expected := []byte("#ps1_sysnative\r\ntest")
	out := renderers.PrependWinPS1Header(in)
	c.Assert(out, jc.DeepEquals, expected)
}

func (s *RenderersSuite) TestAddPowershellTags(c *gc.C) {
	in := []byte("test")
	expected := []byte(`<powershell>` + string(in) + `</powershell>`)
	out := renderers.AddPowershellTags(in)
	c.Assert(out, jc.DeepEquals, expected)
}

func (s *RenderersSuite) TestRenderYAML(c *gc.C) {
	cloudcfg := &cloudinittest.CloudConfig{YAML: []byte("yaml")}
	d1 := func(in []byte) []byte {
		return []byte("1." + string(in))
	}
	d2 := func(in []byte) []byte {
		return []byte("2." + string(in))
	}
	out, err := renderers.RenderYAML(cloudcfg, d2, d1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), jc.DeepEquals, "1.2.yaml")
	cloudcfg.CheckCallNames(c, "RenderYAML")
}

func (s *RenderersSuite) TestRenderScript(c *gc.C) {
	cloudcfg := &cloudinittest.CloudConfig{Script: "script"}
	d1 := func(in []byte) []byte {
		return []byte("1." + string(in))
	}
	d2 := func(in []byte) []byte {
		return []byte("2." + string(in))
	}
	out, err := renderers.RenderScript(cloudcfg, d2, d1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), jc.DeepEquals, "1.2.script")
	cloudcfg.CheckCallNames(c, "RenderScript")
}
