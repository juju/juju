// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package renderers_test

import (
	"encoding/base64"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/cloudconfig/cloudinit/cloudinittest"
	"github.com/juju/juju/internal/cloudconfig/providerinit/renderers"
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
