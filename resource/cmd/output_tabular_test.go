// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

var _ = gc.Suite(&OutputTabularSuite{})

type OutputTabularSuite struct {
	testing.IsolationSuite
}

func (s *OutputTabularSuite) TestFormatTabularOkay(c *gc.C) {
	info := newCharmResource(c, "spam", ".tgz", "...", "")
	formatted := formatInfos(info)

	data, err := FormatTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
RESOURCE FROM   REV COMMENT 
spam     upload -   ...     
`[1:])
}

func (s *OutputTabularSuite) TestFormatTabularMinimal(c *gc.C) {
	info := newCharmResource(c, "spam", "", "", "")
	formatted := formatInfos(info)

	data, err := FormatTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
RESOURCE FROM   REV COMMENT 
spam     upload -           
`[1:])
}

func (s *OutputTabularSuite) TestFormatTabularMulti(c *gc.C) {
	formatted := formatInfos(
		newCharmResource(c, "spam", ".tgz", "spamspamspamspam", ""),
		newCharmResource(c, "eggs", "", "...", ""),
		newCharmResource(c, "somethingbig", ".zip", "", ""),
		newCharmResource(c, "song", ".mp3", "your favorite", ""),
		newCharmResource(c, "avatar", ".png", "your picture", ""),
	)

	data, err := FormatTabular(formatted)
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
	_, err := FormatTabular(bogus)

	c.Check(err, gc.ErrorMatches, `expected value of type .*`)
}

func formatInfos(resources ...charmresource.Resource) []FormattedCharmResource {
	var formatted []FormattedCharmResource
	for _, res := range resources {
		formatted = append(formatted, FormatCharmResource(res))
	}
	return formatted
}
