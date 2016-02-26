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
	"github.com/juju/juju/resource/resourcetesting"
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

func (s *ClientSuite) TestAsRepo(c *gc.C) {
	uuidVal, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	uuid := uuidVal.String()
	base := csclient.New(s.config)
	expected := charmrepo.NewCharmStoreFromClient(base)
	client := charmstore.WrapBaseClient(base, s.client)

	repo := client.AsRepo(uuid)

	c.Check(repo.(*charmrepo.CharmStore), jc.DeepEquals, expected)
}

func (s *ClientSuite) TestLatestCharmInfo(c *gc.C) {
	uuidVal, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	uuid := uuidVal.String()
	cURLs := []*charm.URL{
		charm.MustParseURL("cs:quantal/spam-17"),
		charm.MustParseURL("cs:quantal/eggs-2"),
		charm.MustParseURL("cs:quantal/ham-1"),
	}
	notFound := errors.New("not found")
	repo := &stubRepo{
		Stub: s.stub,
		ReturnLatest: []charmrepo.CharmRevision{{
			Revision: 17,
		}, {
			Revision: 3,
		}, {
			Err: notFound,
		}},
	}
	s.client.ReturnListResources = [][]charmresource.Resource{
		{
			resourcetesting.NewCharmResource(c, "spam", "<some data>"),
		},
		nil,
		nil,
	}
	s.client.ReturnAsRepo = repo

	results, err := charmstore.LatestCharmInfo(s.client, uuid, cURLs)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "AsRepo", "Latest", "ListResources")
	s.stub.CheckCall(c, 0, "AsRepo", uuid)
	s.stub.CheckCall(c, 1, "Latest", cURLs)
	s.stub.CheckCall(c, 2, "ListResources", cURLs)
	timestamp := results[0].Timestamp
	results[2].Error = errors.Cause(results[2].Error)
	c.Check(results, jc.DeepEquals, []charmstore.CharmInfoResult{{
		CharmInfo: charmstore.CharmInfo{
			OriginalURL:    charm.MustParseURL("cs:quantal/spam-17"),
			Timestamp:      timestamp,
			LatestRevision: 17,
			LatestResources: []charmresource.Resource{
				resourcetesting.NewCharmResource(c, "spam", "<some data>"),
			},
		},
	}, {
		CharmInfo: charmstore.CharmInfo{
			OriginalURL:    charm.MustParseURL("cs:quantal/eggs-2"),
			Timestamp:      timestamp,
			LatestRevision: 3,
		},
	}, {
		CharmInfo: charmstore.CharmInfo{
			OriginalURL: charm.MustParseURL("cs:quantal/ham-1"),
			Timestamp:   timestamp,
		},
		Error: notFound,
	}})
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
	charmstore.Client
	*testing.Stub

	ReturnListResources [][]charmresource.Resource
	ReturnAsRepo        charmstore.Repo
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

func (s *stubClient) AsRepo(modelUUID string) charmstore.Repo {
	s.AddCall("AsRepo", modelUUID)
	s.NextErr() // Pop one off.

	return s.ReturnAsRepo
}

type stubRepo struct {
	charmstore.Repo
	*testing.Stub

	ReturnLatest []charmrepo.CharmRevision
}

func (s *stubRepo) Latest(cURLs ...*charm.URL) ([]charmrepo.CharmRevision, error) {
	s.AddCall("Latest", cURLs)
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnLatest, nil
}
