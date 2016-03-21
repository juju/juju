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

	"github.com/juju/juju/charmstore"
)

type ClientSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	client *stubClient
	config charmstore.ClientConfig
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.client = &stubClient{Stub: s.stub}
	s.config = charmstore.ClientConfig{
		charmrepo.NewCharmStoreParams{
			URL: "<something>",
		},
	}
}

func (s *ClientSuite) TestWithMetadata(c *gc.C) {
	uuidVal, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	meta := charmstore.JujuMetadata{
		ModelUUID: uuidVal.String(),
	}
	client := charmstore.NewClient(s.config)
	metaBefore := client.Metadata()

	newClient, err := client.WithMetadata(meta)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(newClient, gc.Not(gc.Equals), client)
	c.Check(metaBefore.IsZero(), jc.IsTrue)
	c.Check(newClient.Metadata(), jc.DeepEquals, meta)
}

func (s *ClientSuite) TestLatestRevisions(c *gc.C) {
	charms := []charmstore.Charm{
		{charm.MustParseURL("cs:quantal/spam-17"), "stable"},
		{charm.MustParseURL("cs:quantal/eggs-2"), "stable"},
		{charm.MustParseURL("cs:quantal/ham-1"), "stable"},
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

	revisions, err := client.LatestRevisions(charms)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "LatestRevisions")
	s.stub.CheckCall(c, 0, "LatestRevisions", charms)
	c.Check(revisions, jc.DeepEquals, expected)
}

// TODO(ericsnow) Move the stub client and repo to a testing package.

type stubClient struct {
	charmstore.BaseClient
	*testing.Stub

	ReturnListResources   [][]charmresource.Resource
	ReturnLatestRevisions []charmrepo.CharmRevision
}

func (s *stubClient) ListResources(charms []charmstore.Charm) ([][]charmresource.Resource, error) {
	s.AddCall("ListResources", charms)
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

func (s *stubClient) LatestRevisions(charms []charmstore.Charm) ([]charmrepo.CharmRevision, error) {
	s.AddCall("LatestRevisions", charms)
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnLatestRevisions, nil
}
