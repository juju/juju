// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package listagreements_test

import (
	"errors"
	"time"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/terms-client/api/wireformat"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/cmd/juju/romulus/listagreements"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&listAgreementsSuite{})

var testTermsAndConditions = "Test Terms and Conditions"

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
	ctx, err := cmdtesting.RunCommand(c, listagreements.NewListAgreementsCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "[]\n")
	c.Assert(s.client.called, jc.IsTrue)

	s.client.setError("well, this is embarassing")
	ctx, err = cmdtesting.RunCommand(c, listagreements.NewListAgreementsCommand())
	c.Assert(err, gc.ErrorMatches, "failed to list user agreements: well, this is embarassing")
	c.Assert(s.client.called, jc.IsTrue)

	agreements := []wireformat.AgreementResponse{{
		User:      "test-user",
		Term:      "test-term",
		Revision:  1,
		CreatedOn: time.Date(2015, 12, 25, 0, 0, 0, 0, time.UTC),
	}}
	s.client.setAgreements(agreements)

	ctx, err = cmdtesting.RunCommand(c, listagreements.NewListAgreementsCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedListAgreementsJSONOutput)
	c.Assert(s.client.called, jc.IsTrue)

	ctx, err = cmdtesting.RunCommand(c, listagreements.NewListAgreementsCommand(), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "- user: test-user\n  term: test-term\n  revision: 1\n  createdon: 2015-12-25T00:00:00Z\n")
	c.Assert(s.client.called, jc.IsTrue)
}

func (s *listAgreementsSuite) TestGetUsersAgreementsWithTermOwner(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, listagreements.NewListAgreementsCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "[]\n")
	c.Assert(s.client.called, jc.IsTrue)

	s.client.setError("well, this is embarassing")
	ctx, err = cmdtesting.RunCommand(c, listagreements.NewListAgreementsCommand())
	c.Assert(err, gc.ErrorMatches, "failed to list user agreements: well, this is embarassing")
	c.Assert(s.client.called, jc.IsTrue)

	agreements := []wireformat.AgreementResponse{{
		User:      "test-user",
		Owner:     "owner",
		Term:      "test-term",
		Revision:  1,
		CreatedOn: time.Date(2015, 12, 25, 0, 0, 0, 0, time.UTC),
	}}
	s.client.setAgreements(agreements)

	ctx, err = cmdtesting.RunCommand(c, listagreements.NewListAgreementsCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedListAgreementsJSONOutputWithOwner)
	c.Assert(s.client.called, jc.IsTrue)

	ctx, err = cmdtesting.RunCommand(c, listagreements.NewListAgreementsCommand(), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "- user: test-user\n  owner: owner\n  term: test-term\n  revision: 1\n  createdon: 2015-12-25T00:00:00Z\n")
	c.Assert(s.client.called, jc.IsTrue)
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

func (c *mockClient) GetUsersAgreements() ([]wireformat.AgreementResponse, error) {
	c.called = true
	if c.err != "" {
		return nil, errors.New(c.err)
	}
	return c.agreements, nil
}
