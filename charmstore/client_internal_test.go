package charmstore

import (
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
)

var _ = gc.Suite(&InternalClientSuite{})

type InternalClientSuite struct {
	testing.IsolationSuite
	lowLevel *fakeWrapper
}

func (s *InternalClientSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.lowLevel = &fakeWrapper{
		stableStub: &testing.Stub{},
		devStub:    &testing.Stub{},
	}
}

func (s *InternalClientSuite) TestCollate(c *gc.C) {
	foo := charm.MustParseURL("cs:quantal/foo-1")
	bar := charm.MustParseURL("cs:quantal/bar-1")
	baz := charm.MustParseURL("cs:quantal/baz-1")
	bat := charm.MustParseURL("cs:quantal/bat-1")
	input := []CharmID{
		{URL: foo, Channel: "stable"},
		{URL: bar, Channel: "development"},
		{URL: baz, Channel: "development"},
		{URL: bat, Channel: "stable"},
	}
	output := collate(input)
	c.Assert(output, gc.DeepEquals, map[params.Channel]charmRequest{
		"stable": charmRequest{
			ids:     []*charm.URL{foo, bat},
			indices: []int{0, 3},
		},
		"development": charmRequest{
			ids:     []*charm.URL{bar, baz},
			indices: []int{1, 2},
		},
	})
}

func (s *InternalClientSuite) TestLatestRevisions(c *gc.C) {
	s.lowLevel.ReturnLatestStable = []charmrepo.CharmRevision{{
		Revision: 1,
		Sha256:   "abc",
	}}
	s.lowLevel.ReturnLatestDev = []charmrepo.CharmRevision{{
		Revision: 2,
		Sha256:   "cde",
	}}

	client := &Client{lowLevel: s.lowLevel}
	foo := charm.MustParseURL("cs:quantal/foo-1")
	bar := charm.MustParseURL("cs:quantal/bar-1")

	ret, err := client.LatestRevisions([]CharmID{{
		URL:     foo,
		Channel: "stable",
	}, {
		URL:     bar,
		Channel: "development",
	}}, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ret, gc.DeepEquals, append(s.lowLevel.ReturnLatestStable, s.lowLevel.ReturnLatestDev...))
	s.lowLevel.stableStub.CheckCall(c, 0, "Latest", params.StableChannel, []*charm.URL{foo}, map[string]string(nil))
	s.lowLevel.devStub.CheckCall(c, 0, "Latest", params.DevelopmentChannel, []*charm.URL{bar}, map[string]string(nil))
}

func (s *InternalClientSuite) TestListResources(c *gc.C) {
	fp, err := resource.GenerateFingerprint(strings.NewReader("data"))
	c.Assert(err, jc.ErrorIsNil)

	stable := params.Resource{
		Name:        "name",
		Type:        "file",
		Path:        "foo.zip",
		Description: "something",
		Origin:      "store",
		Revision:    5,
		Fingerprint: fp.Bytes(),
		Size:        4,
	}
	dev := params.Resource{
		Name:        "name2",
		Type:        "file",
		Path:        "bar.zip",
		Description: "something",
		Origin:      "store",
		Revision:    7,
		Fingerprint: fp.Bytes(),
		Size:        4,
	}
	s.lowLevel.ReturnListResourcesStable = map[string][]params.Resource{
		"cs:quantal/foo-1": []params.Resource{stable},
	}
	s.lowLevel.ReturnListResourcesDev = map[string][]params.Resource{
		"cs:quantal/bar-1": []params.Resource{dev},
	}

	client := Client{lowLevel: s.lowLevel}
	foo := charm.MustParseURL("cs:quantal/foo-1")
	bar := charm.MustParseURL("cs:quantal/bar-1")

	ret, err := client.ListResources([]CharmID{{
		URL:     foo,
		Channel: "stable",
	}, {
		URL:     bar,
		Channel: "development",
	}})
	c.Assert(err, jc.ErrorIsNil)

	stableOut, err := params.API2Resource(stable)
	c.Assert(err, jc.ErrorIsNil)

	devOut, err := params.API2Resource(dev)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(ret, gc.DeepEquals, [][]resource.Resource{
		{stableOut},
		{devOut},
	})
	s.lowLevel.stableStub.CheckCall(c, 0, "ListResources", params.StableChannel, []*charm.URL{foo})
	s.lowLevel.devStub.CheckCall(c, 0, "ListResources", params.DevelopmentChannel, []*charm.URL{bar})
}
