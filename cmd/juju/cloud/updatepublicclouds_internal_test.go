// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	stdtesting "testing"

	"github.com/juju/tc"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/internal/testing"
)

type cloudChangesSuite struct {
	testing.BaseSuite
}

func TestCloudChangesSuite(t *stdtesting.T) {
	tc.Run(t, &cloudChangesSuite{})
}

func (s *cloudChangesSuite) TestPluralityNone(c *tc.C) {
	c.Assert(adjustPlurality("item", 0), tc.Equals, "")
}

func (s *cloudChangesSuite) TestPluralitySingular(c *tc.C) {
	c.Assert(adjustPlurality("item", 1), tc.Equals, "1 item")
}

func (s *cloudChangesSuite) TestPluralityPlural(c *tc.C) {
	c.Assert(adjustPlurality("item", 2), tc.Equals, "2 items")
}

func (s *cloudChangesSuite) TestFormatSliceEmpty(c *tc.C) {
	c.Assert(formatSlice(nil, "", ""), tc.Equals, "")
	c.Assert(formatSlice([]string{}, "", ""), tc.Equals, "")
}

func (s *cloudChangesSuite) TestFormatSliceOne(c *tc.C) {
	c.Assert(formatSlice([]string{"one"}, "", ""), tc.Equals, "one")
}

func (s *cloudChangesSuite) TestFormatSliceTwo(c *tc.C) {
	c.Assert(formatSlice([]string{"one", "two"}, "", " and "), tc.Equals, "one and two")
}

func (s *cloudChangesSuite) TestFormatSliceMany(c *tc.C) {
	c.Assert(formatSlice([]string{"one", "two", "three"}, ", ", " and "), tc.Equals, "one, two and three")
}

func (s *cloudChangesSuite) TestFormatSlices(c *tc.C) {
	c.Assert(formatSlice(
		[]string{"one add", "two and three updates", "four, five and seven deletes"}, "; ", " as well as "),
		tc.Equals,
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
		new:         map[string]jujucloud.Cloud{"one": {Name: "one"}},
		expected: `
1 cloud added:

    added cloud:
        - one`[1:],
	}, {
		description: "added 2 cloud",
		old:         map[string]jujucloud.Cloud{},
		new: map[string]jujucloud.Cloud{
			"one": {Name: "one"},
			"two": {Name: "two"},
		},
		expected: `
2 clouds added:

    added cloud:
        - one
        - two`[1:],
	}, {
		description: "deleted 1 cloud",
		old:         map[string]jujucloud.Cloud{"one": {Name: "one"}},
		new:         map[string]jujucloud.Cloud{},
		expected: `
1 cloud deleted:

    deleted cloud:
        - one`[1:],
	}, {
		description: "deleted 2 cloud",
		old: map[string]jujucloud.Cloud{
			"one": {Name: "one"},
			"two": {Name: "two"},
		},
		new: map[string]jujucloud.Cloud{},
		expected: `
2 clouds deleted:

    deleted cloud:
        - one
        - two`[1:],
	}, {
		description: "cloud attributes change: endpoint",
		old: map[string]jujucloud.Cloud{"one": {
			Name: "one",
		}},
		new: map[string]jujucloud.Cloud{"one": {
			Name:     "one",
			Endpoint: "old_endpoint",
		}},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: identity endpoint",
		old: map[string]jujucloud.Cloud{"one": {
			Name: "one",
		}},
		new: map[string]jujucloud.Cloud{"one": {
			Name:             "one",
			IdentityEndpoint: "old_endpoint"},
		},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: storage endpoint",
		old: map[string]jujucloud.Cloud{"one": {
			Name: "one",
		}},
		new: map[string]jujucloud.Cloud{"one": {
			Name:            "one",
			StorageEndpoint: "old_endpoint"},
		},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: type",
		old: map[string]jujucloud.Cloud{"one": {
			Name: "one",
		}},
		new: map[string]jujucloud.Cloud{"one": {
			Name: "one",
			Type: "type",
		}},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: auth type added",
		old: map[string]jujucloud.Cloud{"one": {
			Name: "one",
		}},
		new: map[string]jujucloud.Cloud{"one": {
			Name:      "one",
			AuthTypes: []jujucloud.AuthType{jujucloud.AccessKeyAuthType}},
		},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: auth type deleted",
		old: map[string]jujucloud.Cloud{"one": {
			Name:      "one",
			AuthTypes: []jujucloud.AuthType{jujucloud.AccessKeyAuthType}},
		},
		new: map[string]jujucloud.Cloud{"one": {
			Name: "one",
		}},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: auth type changed",
		old: map[string]jujucloud.Cloud{"one": {
			Name:      "one",
			AuthTypes: []jujucloud.AuthType{jujucloud.AccessKeyAuthType}},
		},
		new: map[string]jujucloud.Cloud{"one": {
			Name:      "one",
			AuthTypes: []jujucloud.AuthType{jujucloud.JSONFileAuthType}},
		},
		expected: `
1 cloud attribute changed:

    changed cloud attribute:
        - one`[1:],
	}, {
		description: "cloud attributes change: region added",
		old: map[string]jujucloud.Cloud{"one": {
			Name: "one",
		}},
		new: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a"}}},
		},
		expected: `
1 cloud region added:

    added cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region deleted",
		old: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a"}}},
		},
		new: map[string]jujucloud.Cloud{"one": {
			Name: "one",
		}},
		expected: `
1 cloud region deleted:

    deleted cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region changed",
		old: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a"}}},
		},
		new: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a", Endpoint: "old_endpoint"}}},
		},
		expected: `
1 cloud region changed:

    changed cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region changed",
		old: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a", Endpoint: "old_endpoint"}}},
		},
		new: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a"}}},
		},
		expected: `
1 cloud region changed:

    changed cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region changed",
		old: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a", Endpoint: "old_endpoint"}}},
		},
		new: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a", Endpoint: "new_endpoint"}}},
		},
		expected: `
1 cloud region changed:

    changed cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region changed",
		old: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a"}}},
		},
		new: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a", StorageEndpoint: "old_endpoint"}}},
		},
		expected: `
1 cloud region changed:

    changed cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region changed",
		old: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a", StorageEndpoint: "old_endpoint"}}},
		},
		new: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a"}}},
		},
		expected: `
1 cloud region changed:

    changed cloud region:
        - one/a`[1:],
	}, {
		description: "cloud attributes change: region changed",
		old: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a", StorageEndpoint: "old_endpoint"}}},
		},
		new: map[string]jujucloud.Cloud{"one": {
			Name:    "one",
			Regions: []jujucloud.Region{{Name: "a", StorageEndpoint: "new_endpoint"}}},
		},
		expected: `
1 cloud region changed:

    changed cloud region:
        - one/a`[1:],
	}, {
		description: "cloud details changed",
		old: map[string]jujucloud.Cloud{"one": {
			Name: "one",
			Type: "type", Regions: []jujucloud.Region{{Name: "a"}}},
		},
		new: map[string]jujucloud.Cloud{"one": {
			Name: "one",
		}},
		expected: `
1 cloud attribute changed as well as 1 cloud region deleted:

    changed cloud attribute:
        - one
    deleted cloud region:
        - one/a`[1:],
	}, {
		description: "cloud details changed, another way",
		old: map[string]jujucloud.Cloud{"one": {
			Name: "one",
		}},
		new: map[string]jujucloud.Cloud{"one": {
			Name: "one",
			Type: "type", Regions: []jujucloud.Region{{Name: "a"}}},
		},
		expected: `
1 cloud region added as well as 1 cloud attribute changed:

    added cloud region:
        - one/a
    changed cloud attribute:
        - one`[1:],
	}, {
		description: "all cloud change types",
		old: map[string]jujucloud.Cloud{
			"one": {
				Name: "one",
				Type: "type", Regions: []jujucloud.Region{{Name: "a"}},
			},
			"three": {Name: "three"}, // deleting
		},
		new: map[string]jujucloud.Cloud{
			"one": {Name: "one"}, // updating
			"two": {Name: "two"}, // adding
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
			"one": {Name: "one"}, // updating
			"two": {Name: "two"}, // deleting
		},
		new: map[string]jujucloud.Cloud{
			"one": {
				Name:    "three",
				Type:    "type",
				Regions: []jujucloud.Region{{Name: "a"}},
			},
			"three": {
				Name: "three",
			}, // adding
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

func (s *cloudChangesSuite) TestDiffClouds(c *tc.C) {
	for i, test := range diffCloudsTests {
		c.Logf("%d: %v", i, test.description)
		c.Check(diffClouds(test.new, test.old), tc.Equals, test.expected)
	}
}
