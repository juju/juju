// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/testing"
)

type cloudChangesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&cloudChangesSuite{})

func (s *cloudChangesSuite) TestPluralityNone(c *gc.C) {
	c.Assert(adjustPlurality("item", 0), gc.Equals, "")
}

func (s *cloudChangesSuite) TestPluralitySingular(c *gc.C) {
	c.Assert(adjustPlurality("item", 1), gc.Equals, "1 item")
}

func (s *cloudChangesSuite) TestPluralityPlural(c *gc.C) {
	c.Assert(adjustPlurality("item", 2), gc.Equals, "2 items")
}

func (s *cloudChangesSuite) TestFormatSliceEmpty(c *gc.C) {
	c.Assert(formatSlice(nil, "", ""), gc.Equals, "")
	c.Assert(formatSlice([]string{}, "", ""), gc.Equals, "")
}

func (s *cloudChangesSuite) TestFormatSliceOne(c *gc.C) {
	c.Assert(formatSlice([]string{"one"}, "", ""), gc.Equals, "one")
}

func (s *cloudChangesSuite) TestFormatSliceTwo(c *gc.C) {
	c.Assert(formatSlice([]string{"one", "two"}, "", " and "), gc.Equals, "one and two")
}

func (s *cloudChangesSuite) TestFormatSliceMany(c *gc.C) {
	c.Assert(formatSlice([]string{"one", "two", "three"}, ", ", " and "), gc.Equals, "one, two and three")
}

func (s *cloudChangesSuite) TestFormatSlices(c *gc.C) {
	c.Assert(formatSlice(
		[]string{"one add", "two and three updates", "four, five and seven deletes"}, "; ", " as well as "),
		gc.Equals,
		"one add; two and three updates as well as four, five and seven deletes",
	)
}

var diffCloudsTests = []struct {
	description string
	new         map[string]jujucloud.Cloud
	old         map[string]jujucloud.Cloud
	expected    string
}{
	{
		description: "no clouds",
		old:         nil,
		new:         nil,
		expected:    "",
	}, {
		description: "empty new clouds, no old clouds",
		old:         nil,
		new:         map[string]jujucloud.Cloud{},
		expected:    "",
	}, {
		description: "no new clouds, empty old clouds",
		old:         map[string]jujucloud.Cloud{},
		new:         nil,
		expected:    "",
	}, {
		description: "both new and old clouds are empty",
		old:         map[string]jujucloud.Cloud{},
		new:         map[string]jujucloud.Cloud{},
		expected:    "",
	}, {
		description: "added 1 cloud",
		old:         map[string]jujucloud.Cloud{},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{}},
		expected: `
1 cloud added:

    added cloud:
        - one`[1:],
	}, {
		description: "added 2 cloud",
		old:         map[string]jujucloud.Cloud{},
		new: map[string]jujucloud.Cloud{
			"one": jujucloud.Cloud{},
			"two": jujucloud.Cloud{},
		},
		expected: `
2 clouds added:

    added cloud:
        - one
        - two`[1:],
	}, {
		description: "deleted 1 cloud",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{}},
		new:         map[string]jujucloud.Cloud{},
		expected: `
1 cloud deleted:

    deleted cloud:
        - one`[1:],
	}, {
		description: "deleted 2 cloud",
		old: map[string]jujucloud.Cloud{
			"one": jujucloud.Cloud{},
			"two": jujucloud.Cloud{},
		},
		new: map[string]jujucloud.Cloud{},
		expected: `
2 clouds deleted:

    deleted cloud:
        - one
        - two`[1:],
	}, {
		description: "cloud attributes change: endpoint",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Endpoint: "old_endpoint"}},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: identity endpoint",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{IdentityEndpoint: "old_endpoint"}},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: storage endpoint",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{StorageEndpoint: "old_endpoint"}},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: type",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Type: "type"}},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: auth type added",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{AuthTypes: []jujucloud.AuthType{jujucloud.AccessKeyAuthType}}},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: auth type deleted",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{AuthTypes: []jujucloud.AuthType{jujucloud.AccessKeyAuthType}}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{}},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: auth type changed",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{AuthTypes: []jujucloud.AuthType{jujucloud.AccessKeyAuthType}}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{AuthTypes: []jujucloud.AuthType{jujucloud.JSONFileAuthType}}},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: region added",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a"}}}},
		expected: `
1 cloud region added:

    added cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region deleted",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a"}}}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{}},
		expected: `
1 cloud region deleted:

    deleted cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region changed",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a"}}}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a", Endpoint: "old_endpoint"}}}},
		expected: `
1 cloud region changed:

    changed cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region changed",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a", Endpoint: "old_endpoint"}}}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a"}}}},
		expected: `
1 cloud region changed:

    changed cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region changed",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a", Endpoint: "old_endpoint"}}}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a", Endpoint: "new_endpoint"}}}},
		expected: `
1 cloud region changed:

    changed cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region changed",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a"}}}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a", StorageEndpoint: "old_endpoint"}}}},
		expected: `
1 cloud region changed:

    changed cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region changed",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a", StorageEndpoint: "old_endpoint"}}}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a"}}}},
		expected: `
1 cloud region changed:

    changed cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region changed",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a", StorageEndpoint: "old_endpoint"}}}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Regions: []jujucloud.Region{jujucloud.Region{Name: "a", StorageEndpoint: "new_endpoint"}}}},
		expected: `
1 cloud region changed:

    changed cloud region:
        - one/a`[1:],
	}, {
		description: "cloud details changed",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Type: "type", Regions: []jujucloud.Region{jujucloud.Region{Name: "a"}}}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{}},
		expected: `
1 cloud attribute changed as well as 1 cloud region deleted:

    changed cloud attribute:
        - one
    deleted cloud region:
        - one/a`[1:],
	}, {
		description: "cloud details changed, another way",
		old:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{}},
		new:         map[string]jujucloud.Cloud{"one": jujucloud.Cloud{Type: "type", Regions: []jujucloud.Region{jujucloud.Region{Name: "a"}}}},
		expected: `
1 cloud region added as well as 1 cloud attribute changed:

    added cloud region:
        - one/a
    changed cloud attribute:
        - one`[1:],
	}, {
		description: "all cloud change types",
		old: map[string]jujucloud.Cloud{
			"one":   jujucloud.Cloud{Type: "type", Regions: []jujucloud.Region{jujucloud.Region{Name: "a"}}},
			"three": jujucloud.Cloud{}, // deleting
		},
		new: map[string]jujucloud.Cloud{
			"one": jujucloud.Cloud{}, // updating
			"two": jujucloud.Cloud{}, // adding
		},
		expected: `
1 cloud added; 1 cloud attribute changed as well as 1 cloud and 1 cloud region deleted:

    added cloud:
        - two
    changed cloud attribute:
        - one
    deleted cloud:
        - three
    deleted cloud region:
        - one/a`[1:],
	}, {
		description: "all cloud change types, another way",
		old: map[string]jujucloud.Cloud{
			"one": jujucloud.Cloud{}, // updating
			"two": jujucloud.Cloud{}, // deleting
		},
		new: map[string]jujucloud.Cloud{
			"one":   jujucloud.Cloud{Type: "type", Regions: []jujucloud.Region{jujucloud.Region{Name: "a"}}},
			"three": jujucloud.Cloud{}, // adding
		},
		expected: `
1 cloud and 1 cloud region added; 1 cloud attribute changed as well as 1 cloud deleted:

    added cloud:
        - three
    added cloud region:
        - one/a
    changed cloud attribute:
        - one
    deleted cloud:
        - two`[1:],
	},
}

func (s *cloudChangesSuite) TestDiffClouds(c *gc.C) {
	for i, test := range diffCloudsTests {
		c.Logf("%d: %v", i, test.description)
		c.Check(diffClouds(test.new, test.old), gc.Equals, test.expected)
	}
}
