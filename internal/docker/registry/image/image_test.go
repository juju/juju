// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package image_test

import (
	"encoding/json"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/docker/registry/image"
)

type imageSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&imageSuite{})

func (s *imageSuite) TestImageInfo(c *gc.C) {
	imageInfo := image.NewImageInfo(semversion.MustParse("2.9.13"))
	dataJSON, err := json.Marshal(imageInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(dataJSON), gc.DeepEquals, `"2.9.13"`)

	dataYAML, err := yaml.Marshal(imageInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(dataYAML), gc.DeepEquals, `2.9.13
`)

	imageInfo = image.NewImageInfo(semversion.Zero)
	c.Assert(imageInfo.AgentVersion(), gc.DeepEquals, semversion.Zero)
	err = json.Unmarshal(dataJSON, &imageInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageInfo.AgentVersion().String(), gc.DeepEquals, `2.9.13`)

	imageInfo = image.NewImageInfo(semversion.Zero)
	c.Assert(imageInfo.AgentVersion(), gc.DeepEquals, semversion.Zero)
	err = yaml.Unmarshal(dataYAML, &imageInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageInfo.AgentVersion().String(), gc.DeepEquals, `2.9.13`)
}
