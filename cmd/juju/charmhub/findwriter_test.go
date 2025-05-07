// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"

	"github.com/juju/tc"
	"github.com/juju/testing"
)

type columnFindSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&columnFindSuite{})

func (s *columnFindSuite) TestColumnNames(c *tc.C) {
	names := DefaultColumns().Names()
	c.Assert(names, tc.DeepEquals, []string{"Name", "Bundle", "Version", "Architectures", "OS", "Supports", "Publisher", "Summary"})
}

func (s *columnFindSuite) TestMakeColumns(c *tc.C) {
	columns, err := MakeColumns(DefaultColumns(), "nb")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(columns.Names(), tc.DeepEquals, []string{"Name", "Bundle"})
}

func (s *columnFindSuite) TestMakeColumnsOutOfOrder(c *tc.C) {
	columns, err := MakeColumns(DefaultColumns(), "vbn")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(columns.Names(), tc.DeepEquals, []string{"Version", "Bundle", "Name"})
}

func (s *columnFindSuite) TestMakeColumnsInvalidAlias(c *tc.C) {
	_, err := MakeColumns(DefaultColumns(), "X")
	c.Assert(err, tc.ErrorMatches, `unexpected column alias 'X'`)
}

type printFindSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&printFindSuite{})

func (s *printFindSuite) TestCharmPrintFind(c *tc.C) {
	fr := getCharmFindResponse()
	ctx := commandContextForTest(c)
	cols := testDefaultColumns()

	fw := makeFindWriter(ctx.Stdout, ctx.Warningf, cols, fr)
	err := fw.Print()
	c.Assert(err, tc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `
Name       Bundle  Version  Architectures  Supports                                Publisher           Summary
wordpress  -       1.0.3    all            ubuntu@18.04                            WordPress Charmers  WordPress is a full featured web blogging
                                                                                                       tool, this charm deploys it.
osm        Y       3.2.3    all            ubuntu@18.04,ubuntu@20.04,ubuntu@22.04  charmed-osm         Single instance OSM bundle.

`[1:]
	c.Assert(obtained, tc.Equals, expected)
}

func (s *printFindSuite) TestCharmPrintFindWithColumns(c *tc.C) {
	fr := getCharmFindResponse()
	ctx := commandContextForTest(c)
	cols, err := MakeColumns(DefaultColumns(), "nbvps")
	c.Assert(err, tc.ErrorIsNil)

	fw := makeFindWriter(ctx.Stdout, ctx.Warningf, cols, fr)
	err = fw.Print()
	c.Assert(err, tc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `
Name       Bundle  Version  Publisher           Summary
wordpress  -       1.0.3    WordPress Charmers  WordPress is a full featured web blogging
                                                tool, this charm deploys it.
osm        Y       3.2.3    charmed-osm         Single instance OSM bundle.

`[1:]
	c.Assert(obtained, tc.Equals, expected)
}

func (s *printFindSuite) TestCharmPrintFindWithMissingData(c *tc.C) {
	fr := getCharmFindResponse()
	fr[0].Version = ""
	fr[0].Arches = make([]string, 0)
	fr[0].Supports = []Base{}
	fr[0].Summary = ""

	ctx := commandContextForTest(c)
	cols := testDefaultColumns()

	fw := makeFindWriter(ctx.Stdout, ctx.Warningf, cols, fr)
	err := fw.Print()
	c.Assert(err, tc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `
Name       Bundle  Version  Architectures  Supports                                Publisher           Summary
wordpress  -                                                                       WordPress Charmers  
osm        Y       3.2.3    all            ubuntu@18.04,ubuntu@20.04,ubuntu@22.04  charmed-osm         Single instance OSM bundle.

`[1:]
	c.Assert(obtained, tc.Equals, expected)
}

func (s *printFindSuite) TestSummary(c *tc.C) {
	summary, err := oneLine("WordPress is a full featured web blogging tool, this charm deploys it.\nSome addition data\nMore Lines", 0)
	c.Assert(err, tc.ErrorIsNil)

	obtained := summary
	expected := `
WordPress is a full featured web blogging
tool, this charm deploys it.`
	c.Assert(obtained, tc.Equals, expected[1:])
}

func (s *printFindSuite) TestSummaryEmpty(c *tc.C) {
	summary, err := oneLine("", 0)
	c.Assert(err, tc.ErrorIsNil)

	obtained := summary
	expected := ""
	c.Assert(obtained, tc.Equals, expected)
}

func getCharmFindResponse() []FindResponse {
	return []FindResponse{{
		Name:      "wordpress",
		Type:      "charm",
		ID:        "charmCHARMcharmCHARMcharmCHARM01",
		Publisher: "WordPress Charmers",
		Summary:   "WordPress is a full featured web blogging tool, this charm deploys it.",
		Version:   "1.0.3",
		Arches:    []string{"all"},
		Supports: []Base{
			{Name: "ubuntu", Channel: "18.04"},
		},
		StoreURL: "https://someurl.com/wordpress",
	}, {
		Name:      "osm",
		Type:      "bundle",
		ID:        "bundleBUNDLEbundleBUNDLEbundleBUNDLE01",
		Publisher: "charmed-osm",
		Summary:   "Single instance OSM bundle.",
		Version:   "3.2.3",
		Arches:    []string{"all"},
		Supports: []Base{
			{Name: "ubuntu", Channel: "18.04"},
			{Name: "ubuntu", Channel: "20.04"},
			{Name: "ubuntu", Channel: "22.04"},
		},
		StoreURL: "https://someurl.com/osm",
	}}
}

func testDefaultColumns() Columns {
	return map[rune]Column{
		'n': {Index: 0, Name: ColumnNameName},
		'b': {Index: 1, Name: ColumnNameBundle},
		'v': {Index: 2, Name: ColumnNameVersion},
		'a': {Index: 3, Name: ColumnNameArchitectures},
		'S': {Index: 4, Name: ColumnNameSupports},
		'p': {Index: 5, Name: ColumnNamePublisher},
		's': {Index: 6, Name: ColumnNameSummary},
	}
}
