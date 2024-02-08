// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package listagreements_test

import (
	"context"
	"errors"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/terms-client/v2/api/wireformat"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/agree/listagreements"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&listAgreementsSuite{})

type listAgreementsSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	client *mockClient
}

func (s *listAgreementsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.client = &mockClient{}

	jujutesting.PatchValue(listagreements.NewClient, func(_ *httpbakery.Client) (listagreements.TermsServiceClient, error) {
		return s.client, nil
	})
}

const (
	expectedListAgreementsTabularOutput = `Term       	                    Agreed on
test-term/1	2015-12-25 00:00:00 +0000 UTC
`

	expectedListAgreementsTabularOutputWithOwner = `Term             	                    Agreed on
owner/test-term/1	2015-12-25 00:00:00 +0000 UTC
`

	expectedListAgreementsJSONOutput = `[
    {
        "user": "test-user",
        "term": "test-term",
        "revision": 1,
        "created-on": "2015-12-25T00:00:00Z"
    }
]
`
	expectedListAgreementsJSONOutputWithOwner = `[
    {
        "user": "test-user",
        "owner": "owner",
        "term": "test-term",
        "revision": 1,
        "created-on": "2015-12-25T00:00:00Z"
    }
]
`
)

func (s *listAgreementsSuite) TestGetUsersAgreements(c *gc.C) {
	ctx, err := s.runCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "No agreements to display.\n")
	c.Assert(s.client.called, jc.IsTrue)

	s.client.setError("well, this is embarrassing")

	_, err = s.runCommand(c)
	c.Assert(err, gc.ErrorMatches, "failed to list user agreements: well, this is embarrassing")
	c.Assert(s.client.called, jc.IsTrue)

	agreements := []wireformat.AgreementResponse{{
		User:      "test-user",
		Term:      "test-term",
		Revision:  1,
		CreatedOn: time.Date(2015, 12, 25, 0, 0, 0, 0, time.UTC),
	}}
	s.client.setAgreements(agreements)

	ctx, err = s.runCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedListAgreementsTabularOutput)
	c.Assert(s.client.called, jc.IsTrue)

	ctx, err = s.runCommand(c, "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "- user: test-user\n  term: test-term\n  revision: 1\n  createdon: 2015-12-25T00:00:00Z\n")
	c.Assert(s.client.called, jc.IsTrue)

	ctx, err = s.runCommand(c, "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedListAgreementsJSONOutput)
	c.Assert(s.client.called, jc.IsTrue)
}

func (s *listAgreementsSuite) TestGetUsersAgreementsWithTermOwner(c *gc.C) {
	s.client.setError("well, this is embarrassing")
	_, err := s.runCommand(c)
	c.Assert(err, gc.ErrorMatches, "failed to list user agreements: well, this is embarrassing")
	c.Assert(s.client.called, jc.IsTrue)

	agreements := []wireformat.AgreementResponse{{
		User:      "test-user",
		Owner:     "owner",
		Term:      "test-term",
		Revision:  1,
		CreatedOn: time.Date(2015, 12, 25, 0, 0, 0, 0, time.UTC),
	}}
	s.client.setAgreements(agreements)

	ctx, err := s.runCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedListAgreementsTabularOutputWithOwner)
	c.Assert(s.client.called, jc.IsTrue)

	ctx, err = s.runCommand(c, "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedListAgreementsJSONOutputWithOwner)
	c.Assert(s.client.called, jc.IsTrue)

	ctx, err = s.runCommand(c, "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "- user: test-user\n  owner: owner\n  term: test-term\n  revision: 1\n  createdon: 2015-12-25T00:00:00Z\n")
	c.Assert(s.client.called, jc.IsTrue)
}

func (s *listAgreementsSuite) runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := listagreements.NewListAgreementsCommand()
	cmd.SetClientStore(newMockStore())
	return cmdtesting.RunCommand(c, cmd, args...)
}

type mockClient struct {
	called bool

	agreements []wireformat.AgreementResponse
	err        string
}

func (c *mockClient) setAgreements(agreements []wireformat.AgreementResponse) {
	c.agreements = agreements
	c.called = false
	c.err = ""
}

func (c *mockClient) setError(err string) {
	c.err = err
	c.called = false
	c.agreements = nil
}

func (c *mockClient) GetUsersAgreements(ctx context.Context) ([]wireformat.AgreementResponse, error) {
	c.called = true
	if c.err != "" {
		return nil, errors.New(c.err)
	}
	return c.agreements, nil
}

func newMockStore() *jujuclient.MemStore {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}
	return store
}
