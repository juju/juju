// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"

	"github.com/juju/juju/charmstore"
)

type ClientSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	client *stubClient
	config csclient.Params
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.client = &stubClient{Stub: s.stub}
	s.config = csclient.Params{
		URL: "<something>",
	}
}

func (s *ClientSuite) TestWrapBaseClient(c *gc.C) {
	base := csclient.New(s.config)

	client := charmstore.WrapBaseClient(base, s.client)
	err := client.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Close")
}

func (s *ClientSuite) TestWithMetadata(c *gc.C) {
	uuidVal, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	meta := charmstore.JujuMetadata{
		ModelUUID: uuidVal.String(),
	}
	base := csclient.New(s.config)
	client := charmstore.WrapBaseClient(base, s.client)
	metaBefore := client.Metadata()

	newClient, err := client.WithMetadata(meta)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(newClient, gc.Not(gc.Equals), client)
	c.Check(metaBefore.IsZero(), jc.IsTrue)
	c.Check(newClient.Metadata(), jc.DeepEquals, meta)
}

func (s *ClientSuite) TestLatestRevisions(c *gc.C) {
	cURLs := []*charm.URL{
		charm.MustParseURL("cs:quantal/spam-17"),
		charm.MustParseURL("cs:quantal/eggs-2"),
		charm.MustParseURL("cs:quantal/ham-1"),
	}
	expected := []charmrepo.CharmRevision{{
		Revision: 17,
	}, {
		Revision: 3,
	}, {
		Err: errors.New("not found"),
	}}
	s.client.ReturnLatestRevisions = expected
	client := charmstore.Client{BaseClient: s.client}

	revisions, err := client.LatestRevisions(cURLs)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "LatestRevisions")
	s.stub.CheckCall(c, 0, "LatestRevisions", cURLs)
	c.Check(revisions, jc.DeepEquals, expected)
}

func (s *ClientSuite) TestFakeListResources(c *gc.C) {
	cURLs := []*charm.URL{
		charm.MustParseURL("cs:quantal/spam-17"),
		charm.MustParseURL("cs:quantal/eggs-2"),
	}
	base := csclient.New(s.config)
	client := charmstore.WrapBaseClient(base, s.client)

	results, err := client.ListResources(cURLs)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, [][]charmresource.Resource{{}, {}})
}

func (s *ClientSuite) TestFakeGetResource(c *gc.C) {
	cURL := charm.MustParseURL("cs:quantal/spam-17")
	base := csclient.New(s.config)
	client := charmstore.WrapBaseClient(base, s.client)

	_, err := client.GetResource(cURL, "spam", 3)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

// TODO(ericsnow) Move the stub client and repo to a testing package.

type stubClient struct {
	charmstore.BaseClient
	*testing.Stub

	ReturnListResources   [][]charmresource.Resource
	ReturnLatestRevisions []charmrepo.CharmRevision
}

func (s *stubClient) ListResources(cURLs []*charm.URL) ([][]charmresource.Resource, error) {
	s.AddCall("ListResources", cURLs)
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

func (s *stubClient) Close() error {
	s.AddCall("Close")
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubClient) LatestRevisions(cURLs []*charm.URL) ([]charmrepo.CharmRevision, error) {
	s.AddCall("LatestRevisions", cURLs)
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnLatestRevisions, nil
}
