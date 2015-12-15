// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource/cmd"
)

var _ = gc.Suite(&OutputTabularSuite{})

type OutputTabularSuite struct {
	testing.IsolationSuite
}

func (s *OutputTabularSuite) TestFormatTabularOkay(c *gc.C) {
	info := cmd.NewCharmResource(c, "spam", ".tgz", "...", "")
	formatted := formatInfos(info)

	data, err := cmd.FormatTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
RESOURCE FROM   REV COMMENT 
spam     upload -   ...     
`[1:])
}

func (s *OutputTabularSuite) TestFormatTabularMinimal(c *gc.C) {
	info := cmd.NewCharmResource(c, "spam", "", "", "")
	formatted := formatInfos(info)

	data, err := cmd.FormatTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
RESOURCE FROM   REV COMMENT 
spam     upload -           
`[1:])
}

func (s *OutputTabularSuite) TestFormatTabularMulti(c *gc.C) {
	formatted := formatInfos(
		cmd.NewCharmResource(c, "spam", ".tgz", "spamspamspamspam", ""),
		cmd.NewCharmResource(c, "eggs", "", "...", ""),
		cmd.NewCharmResource(c, "somethingbig", ".zip", "", ""),
		cmd.NewCharmResource(c, "song", ".mp3", "your favorite", ""),
		cmd.NewCharmResource(c, "avatar", ".png", "your picture", ""),
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
	bogus := "should have been []formattedInfo"
	_, err := cmd.FormatTabular(bogus)

	c.Check(err, gc.ErrorMatches, `expected value of type .*`)
}

func formatInfos(resources ...charmresource.Resource) []cmd.FormattedCharmResource {
	var formatted []cmd.FormattedCharmResource
	for _, res := range resources {
		formatted = append(formatted, cmd.FormatCharmResource(res))
	}
	return formatted
}
