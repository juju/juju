// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/cmd"
)

var _ = gc.Suite(&OutputTabularSuite{})

type OutputTabularSuite struct {
	testing.IsolationSuite
}

func (s *OutputTabularSuite) TestFormatTabularOkay(c *gc.C) {
	spec := cmd.NewSpec(c, "spam", ".tgz", "...")
	formatted := formatSpecs(spec)

	data, err := cmd.FormatTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
RESOURCE FROM   REV COMMENT 
spam     upload -   ...     
`[1:])
}

func (s *OutputTabularSuite) TestFormatTabularMinimal(c *gc.C) {
	spec := cmd.NewSpec(c, "spam", "", "")
	formatted := formatSpecs(spec)

	data, err := cmd.FormatTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
RESOURCE FROM   REV COMMENT 
spam     upload -           
`[1:])
}

func (s *OutputTabularSuite) TestFormatTabularMulti(c *gc.C) {
	formatted := formatSpecs(
		cmd.NewSpec(c, "spam", ".tgz", "spamspamspamspam"),
		cmd.NewSpec(c, "eggs", "", "..."),
		cmd.NewSpec(c, "somethingbig", ".zip", ""),
		cmd.NewSpec(c, "song", ".mp3", "your favorite"),
		cmd.NewSpec(c, "avatar", ".png", "your picture"),
	)

	data, err := cmd.FormatTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
RESOURCE     FROM   REV COMMENT          
spam         upload -   spamspamspamspam 
eggs         upload -   ...              
somethingbig upload -                    
song         upload -   your favorite    
avatar       upload -   your picture     
`[1:])
}

func (s *OutputTabularSuite) TestFormatTabularBadValue(c *gc.C) {
	bogus := "should have been []formattedSpec"
	_, err := cmd.FormatTabular(bogus)

	c.Check(err, gc.ErrorMatches, `expected value of type .*`)
}

func formatSpecs(specs ...resource.Spec) []cmd.FormattedSpec {
	var formatted []cmd.FormattedSpec
	for _, spec := range specs {
		formatted = append(formatted, cmd.FormatSpec(spec))
	}
	return formatted
}
