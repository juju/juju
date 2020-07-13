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
Name       Bundle  Version  Supports              Publisher          Summary
wordpress  -       1.0.3    bionic                Wordress Charmers  WordPress is a full featured web blogging tool, this charm deploys it.
osm        Y       3.2.3    bionic,xenial,trusty  charmed-osm        Single instance OSM bundle.

`[1:]
	c.Assert(obtained, gc.Equals, expected)
}

func (s *printFindSuite) TestCharmPrintFindWithMissingData(c *gc.C) {
	fr := getCharmFindResponse()
	fr[0].Version = ""
	fr[0].Series = make([]string, 0)
	fr[0].Summary = ""

	ctx := commandContextForTest(c)
	fw := makeFindWriter(ctx, fr)
	err := fw.Print()
	c.Assert(err, jc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `
Name       Bundle  Version  Supports              Publisher          Summary
wordpress  -                                      Wordress Charmers  
osm        Y       3.2.3    bionic,xenial,trusty  charmed-osm        Single instance OSM bundle.

`[1:]
	c.Assert(obtained, gc.Equals, expected)
}

func (s *printFindSuite) TestSummary(c *gc.C) {
	summary, err := oneLine("WordPress is a full featured web blogging tool, this charm deploys it.\nSome addition data\nMore Lines")
	c.Assert(err, jc.ErrorIsNil)

	obtained := summary
	expected := "WordPress is a full featured web blogging tool, this charm deploys it."
	c.Assert(obtained, gc.Equals, expected)
}

func (s *printFindSuite) TestSummaryEmpty(c *gc.C) {
	summary, err := oneLine("")
	c.Assert(err, jc.ErrorIsNil)

	obtained := summary
	expected := ""
	c.Assert(obtained, gc.Equals, expected)
}

func getCharmFindResponse() []charmhub.FindResponse {
	return []charmhub.FindResponse{{
		Name:      "wordpress",
		Type:      "charm",
		ID:        "charmCHARMcharmCHARMcharmCHARM01",
		Publisher: "Wordress Charmers",
		Summary:   "WordPress is a full featured web blogging tool, this charm deploys it.",
		Version:   "1.0.3",
		Series:    []string{"bionic"},
		StoreURL:  "https://someurl.com/wordpress",
	}, {
		Name:      "osm",
		Type:      "bundle",
		ID:        "bundleBUNDLEbundleBUNDLEbundleBUNDLE01",
		Publisher: "charmed-osm",
		Summary:   "Single instance OSM bundle.",
		Version:   "3.2.3",
		Series:    []string{"bionic", "xenial", "trusty"},
		StoreURL:  "https://someurl.com/osm",
	}}
}
