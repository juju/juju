// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"

	"github.com/juju/juju/api/charmhub"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type printFindSuite struct{}

var _ = gc.Suite(&printFindSuite{})

func (s *printFindSuite) TestCharmPrintFind(c *gc.C) {
	fr := getCharmFindResponse()
	ctx := commandContextForTest(c)
	fw := makeFindWriter(ctx, fr)
	err := fw.Print()
	c.Assert(err, jc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `
Name       Version  Publisher           Notes  Summary
wordpress  1.0.3    Wordpress Charmers  -      WordPress is a full featured web blogging tool, this charm deploys it.

`[1:]
	c.Assert(obtained, gc.Equals, expected)
}

func (s *printFindSuite) TestCharmPrintFindWithMissingData(c *gc.C) {
	fr := getCharmFindResponse()
	fr[0].Entity.Summary = ""
	fr[0].Entity.Publisher = make(map[string]string)
	fr[0].DefaultRelease = charmhub.ChannelMap{}

	ctx := commandContextForTest(c)
	fw := makeFindWriter(ctx, fr)
	err := fw.Print()
	c.Assert(err, jc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `
Name       Version  Publisher  Notes  Summary
wordpress                      -      

`[1:]
	c.Assert(obtained, gc.Equals, expected)
}

func (s *printFindSuite) TestPublisher(c *gc.C) {
	entity := getCharmEntity()
	fw := findWriter{}
	publisher := fw.publisher(entity)

	obtained := publisher
	expected := "Wordpress Charmers"
	c.Assert(obtained, gc.Equals, expected)
}

func (s *printFindSuite) TestPublisherEmpty(c *gc.C) {
	entity := getCharmEntity()
	entity.Publisher = make(map[string]string)

	fw := findWriter{}
	publisher := fw.publisher(entity)

	obtained := publisher
	expected := ""
	c.Assert(obtained, gc.Equals, expected)
}

func (s *printFindSuite) TestSummary(c *gc.C) {
	entity := getCharmEntity()
	fw := findWriter{}
	summary := fw.summary(entity)

	obtained := summary
	expected := "WordPress is a full featured web blogging tool, this charm deploys it."
	c.Assert(obtained, gc.Equals, expected)
}

func (s *printFindSuite) TestSummaryEmpty(c *gc.C) {
	entity := getCharmEntity()
	entity.Summary = ""

	fw := findWriter{}
	summary := fw.summary(entity)

	obtained := summary
	expected := ""
	c.Assert(obtained, gc.Equals, expected)
}

func getCharmFindResponse() []charmhub.FindResponse {
	return []charmhub.FindResponse{{
		Name:   "wordpress",
		Type:   "charm",
		ID:     "charmCHARMcharmCHARMcharmCHARM01",
		Entity: getCharmEntity(),
		DefaultRelease: charmhub.ChannelMap{
			Channel: charmhub.Channel{
				Name: "latest/stable",
				Platform: charmhub.Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
			},
			Revision: charmhub.Revision{
				ConfigYAML: config,
				CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
				Download: charmhub.Download{
					HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
					Size:       12042240,
					URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
				},
				MetadataYAML: "name: myname\nversion: 1.0.3\nsubordinate: false\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\ntags: [app, seven]\nseries: [bionic, xenial]\n",
				Revision:     16,
				Version:      "1.0.3",
			},
		},
	}}
}

func getCharmEntity() charmhub.Entity {
	return charmhub.Entity{
		Categories: []charmhub.Category{{
			Featured: true,
			Name:     "blog",
		}},
		Description: "This will install and setup WordPress optimized to run in the cloud. By default it will place Ngnix and php-fpm configured to scale horizontally with Nginx's reverse proxy.",
		Publisher: map[string]string{
			"display-name": "Wordpress Charmers",
		},
		Summary: "WordPress is a full featured web blogging tool, this charm deploys it.\nFor blogging.",
	}
}
