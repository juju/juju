// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package image_test

import (
	"encoding/json"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/docker/registry/image"
)

type imageSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&imageSuite{})

func (s *imageSuite) TestImageInfo(c *tc.C) {
	imageInfo := image.NewImageInfo(semversion.MustParse("2.9.13"))
	dataJSON, err := json.Marshal(imageInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(dataJSON), tc.DeepEquals, `"2.9.13"`)

	dataYAML, err := yaml.Marshal(imageInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(dataYAML), tc.DeepEquals, `2.9.13
`)

	imageInfo = image.NewImageInfo(semversion.Zero)
	c.Assert(imageInfo.AgentVersion(), tc.DeepEquals, semversion.Zero)
	err = json.Unmarshal(dataJSON, &imageInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageInfo.AgentVersion().String(), tc.DeepEquals, `2.9.13`)

	imageInfo = image.NewImageInfo(semversion.Zero)
	c.Assert(imageInfo.AgentVersion(), tc.DeepEquals, semversion.Zero)
	err = yaml.Unmarshal(dataYAML, &imageInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageInfo.AgentVersion().String(), tc.DeepEquals, `2.9.13`)
}
