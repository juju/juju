// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"io/ioutil"
	"strings"

	"github.com/juju/charm/v7"
	"github.com/juju/charm/v7/resource"
	"github.com/juju/charmrepo/v5/csclient"
	"github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&ClientSuite{})

type ClientSuite struct {
	testing.IsolationSuite
	wrapper *fakeWrapper
	cache   *fakeMacCache
}

func (s *ClientSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.wrapper = &fakeWrapper{
		stub:       &testing.Stub{},
		stableStub: &testing.Stub{},
		devStub:    &testing.Stub{},
	}

	s.cache = &fakeMacCache{
		stub: &testing.Stub{},
	}
}

func (s *ClientSuite) TestLatestRevisions(c *gc.C) {
	s.wrapper.ReturnLatestStable = [][]params.CharmRevision{{{
		Revision: 1,
		Sha256:   "abc",
	}}}
	s.wrapper.ReturnLatestDev = [][]params.CharmRevision{{{
		Revision: 2,
		Sha256:   "cde",
	}}, {{
		Revision: 3,
		Sha256:   "fgh",
	}}}

	client, err := newCachingClient(s.cache, "", s.wrapper.makeWrapper)
	c.Assert(err, jc.ErrorIsNil)

	foo := charm.MustParseURL("cs:quantal/foo-1")
	bar := charm.MustParseURL("cs:quantal/bar-1")
	baz := charm.MustParseURL("cs:quantal/baz-1")

	ret, err := client.LatestRevisions([]CharmID{{
		URL:     foo,
		Channel: params.StableChannel,
	}, {
		URL:     bar,
		Channel: params.EdgeChannel,
	}, {
		URL:     baz,
		Channel: params.EdgeChannel,
	}}, nil)
	c.Assert(err, jc.ErrorIsNil)
	expected := []CharmRevision{{
		Revision: 1,
	}, {
		Revision: 2,
	}, {
		Revision: 3,
	}}
	c.Check(ret, jc.SameContents, expected)
	s.wrapper.stableStub.CheckCall(c, 0, "Latest", params.StableChannel, []*charm.URL{foo}, map[string][]string(nil))
	s.wrapper.devStub.CheckCall(c, 0, "Latest", params.EdgeChannel, []*charm.URL{bar}, map[string][]string(nil))
	s.wrapper.devStub.CheckCall(c, 1, "Latest", params.EdgeChannel, []*charm.URL{baz}, map[string][]string(nil))
}

func (s *ClientSuite) TestListResources(c *gc.C) {
	fp, err := resource.GenerateFingerprint(strings.NewReader("data"))
	c.Assert(err, jc.ErrorIsNil)

	stable := params.Resource{
		Name:        "name",
		Type:        "file",
		Path:        "foo.zip",
		Description: "something",
		Revision:    5,
		Fingerprint: fp.Bytes(),
		Size:        4,
	}
	dev := params.Resource{
		Name:        "name2",
		Type:        "file",
		Path:        "bar.zip",
		Description: "something",
		Revision:    7,
		Fingerprint: fp.Bytes(),
		Size:        4,
	}
	dev2 := params.Resource{
		Name:        "name3",
		Type:        "file",
		Path:        "bar.zip",
		Description: "something",
		Revision:    8,
		Fingerprint: fp.Bytes(),
		Size:        4,
	}

	s.wrapper.ReturnListResourcesStable = []resourceResult{oneResourceResult(stable), {err: params.ErrNotFound}}
	s.wrapper.ReturnListResourcesDev = []resourceResult{oneResourceResult(dev), oneResourceResult(dev2)}

	client, err := newCachingClient(s.cache, "", s.wrapper.makeWrapper)
	c.Assert(err, jc.ErrorIsNil)

	foo := charm.MustParseURL("cs:quantal/foo-1")
	bar := charm.MustParseURL("cs:quantal/bar-1")
	baz := charm.MustParseURL("cs:quantal/baz-1")

	ret, err := client.ListResources([]CharmID{{
		URL:     foo,
		Channel: params.StableChannel,
	}, {
		URL:     bar,
		Channel: params.EdgeChannel,
	}, {
		URL:     baz,
		Channel: params.EdgeChannel,
	}})
	c.Assert(err, jc.ErrorIsNil)

	stableOut, err := params.API2Resource(stable)
	c.Assert(err, jc.ErrorIsNil)

	devOut, err := params.API2Resource(dev)
	c.Assert(err, jc.ErrorIsNil)

	dev2Out, err := params.API2Resource(dev2)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(ret, gc.DeepEquals, [][]resource.Resource{
		{stableOut},
		{devOut},
		{dev2Out},
	})
	s.wrapper.stableStub.CheckCall(c, 0, "ListResources", params.StableChannel, foo)
	s.wrapper.devStub.CheckCall(c, 0, "ListResources", params.EdgeChannel, bar)
	s.wrapper.devStub.CheckCall(c, 1, "ListResources", params.EdgeChannel, baz)
}

func (s *ClientSuite) TestListResourcesError(c *gc.C) {
	s.wrapper.ReturnListResourcesStable = []resourceResult{{err: errors.NotFoundf("another error")}}
	client, err := newCachingClient(s.cache, "", s.wrapper.makeWrapper)
	c.Assert(err, jc.ErrorIsNil)

	ret, err := client.ListResources([]CharmID{{
		URL:     charm.MustParseURL("cs:quantal/foo-1"),
		Channel: params.StableChannel,
	}})
	c.Assert(err, gc.ErrorMatches, `another error not found`)
	c.Assert(ret, gc.IsNil)
}

func (s *ClientSuite) TestGetResource(c *gc.C) {
	fp, err := resource.GenerateFingerprint(strings.NewReader("data"))
	c.Assert(err, jc.ErrorIsNil)
	rc := ioutil.NopCloser(strings.NewReader("data"))
	s.wrapper.ReturnGetResource = csclient.ResourceData{
		ReadCloser: rc,
		Hash:       fp.String(),
	}
	apiRes := params.Resource{
		Name:        "name",
		Type:        "file",
		Path:        "foo.zip",
		Description: "something",
		Revision:    5,
		Fingerprint: fp.Bytes(),
		Size:        4,
	}
	s.wrapper.ReturnResourceMeta = apiRes

	client, err := newCachingClient(s.cache, "", s.wrapper.makeWrapper)
	c.Assert(err, jc.ErrorIsNil)

	req := ResourceRequest{
		Charm:    charm.MustParseURL("cs:mysql"),
		Channel:  params.EdgeChannel,
		Name:     "name",
		Revision: 5,
	}
	data, err := client.GetResource(req)
	c.Assert(err, jc.ErrorIsNil)
	expected, err := params.API2Resource(apiRes)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(data.Resource, gc.DeepEquals, expected)
	c.Check(data.ReadCloser, gc.DeepEquals, rc)
	// call #0 is a call to makeWrapper
	s.wrapper.stub.CheckCall(c, 1, "ResourceMeta", params.EdgeChannel, req.Charm, req.Name, req.Revision)
	s.wrapper.stub.CheckCall(c, 2, "GetResource", params.EdgeChannel, req.Charm, req.Name, req.Revision)
}

func (s *ClientSuite) TestGetResourceDockerType(c *gc.C) {
	fp, err := resource.GenerateFingerprint(strings.NewReader("data"))
	c.Assert(err, jc.ErrorIsNil)
	rc := ioutil.NopCloser(strings.NewReader("data"))
	s.wrapper.ReturnGetResource = csclient.ResourceData{
		ReadCloser: rc,
		Hash:       fp.String(),
	}
	apiRes := params.Resource{
		Name:        "mysql_image",
		Type:        "oci-image",
		Description: "something",
		Revision:    2,
		Fingerprint: resource.Fingerprint{}.Bytes(),
		Size:        4,
	}
	s.wrapper.ReturnResourceMeta = apiRes

	client, err := newCachingClient(s.cache, "", s.wrapper.makeWrapper)
	c.Assert(err, jc.ErrorIsNil)

	req := ResourceRequest{
		Charm:    charm.MustParseURL("cs:mysql"),
		Channel:  params.EdgeChannel,
		Name:     "mysql_image",
		Revision: 5,
	}
	data, err := client.GetResource(req)
	c.Assert(err, jc.ErrorIsNil)
	expected, err := params.API2Resource(apiRes)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(data.Resource, gc.DeepEquals, expected)
	c.Check(data.ReadCloser, gc.DeepEquals, rc)
	// call #0 is a call to makeWrapper
	s.wrapper.stub.CheckCall(c, 1, "ResourceMeta", params.EdgeChannel, req.Charm, req.Name, req.Revision)
	s.wrapper.stub.CheckCall(c, 2, "GetResource", params.EdgeChannel, req.Charm, req.Name, req.Revision)
}

func (s *ClientSuite) TestResourceInfo(c *gc.C) {
	fp, err := resource.GenerateFingerprint(strings.NewReader("data"))
	c.Assert(err, jc.ErrorIsNil)
	apiRes := params.Resource{
		Name:        "name",
		Type:        "file",
		Path:        "foo.zip",
		Description: "something",
		Revision:    5,
		Fingerprint: fp.Bytes(),
		Size:        4,
	}
	s.wrapper.ReturnResourceMeta = apiRes

	client, err := newCachingClient(s.cache, "", s.wrapper.makeWrapper)
	c.Assert(err, jc.ErrorIsNil)

	req := ResourceRequest{
		Charm:    charm.MustParseURL("cs:mysql"),
		Channel:  params.StableChannel,
		Name:     "name",
		Revision: 5,
	}
	res, err := client.ResourceInfo(req)
	c.Assert(err, jc.ErrorIsNil)
	expected, err := params.API2Resource(apiRes)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, expected)
	// call #0 is a call to makeWrapper
	s.wrapper.stub.CheckCall(c, 1, "ResourceMeta", params.StableChannel, req.Charm, req.Name, req.Revision)
}
