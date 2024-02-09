// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agree_test

import (
	"context"
	"sync"
	"testing"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/terms-client/v2/api"
	"github.com/juju/terms-client/v2/api/wireformat"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/agree/agree"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&agreeSuite{})

var testTerms = "Test Terms"

type agreeSuite struct {
	client *mockClient
	coretesting.FakeJujuXDGDataHomeSuite
}

func (s *agreeSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.client = &mockClient{}

	jujutesting.PatchValue(agree.ClientNew, func(...api.ClientOption) (api.Client, error) {
		return s.client, nil
	})
}

func (s *agreeSuite) TestAgreementNothingToSign(c *gc.C) {
	jujutesting.PatchValue(agree.UserAnswer, func() (string, error) {
		return "y", nil
	})

	s.client.user = "test-user"
	s.client.setUnsignedTerms([]wireformat.GetTermsResponse{})

	ctx, err := s.runCommand(c, "test-term/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `Already agreed
`)
}

func (s *agreeSuite) TestAgreement(c *gc.C) {
	var answer string
	jujutesting.PatchValue(agree.UserAnswer, func() (string, error) {
		return answer, nil
	})

	s.client.user = "test-user"
	s.client.setUnsignedTerms([]wireformat.GetTermsResponse{{
		Name:     "test-term",
		Revision: 1,
		Content:  testTerms,
	}})
	tests := []struct {
		about       string
		args        []string
		err         string
		stdout      string
		answer      string
		apiCalls    []jujutesting.StubCall
		clientTerms []wireformat.GetTermsResponse
	}{{
		about:    "everything works",
		args:     []string{"test-term/1", "--yes"},
		stdout:   "Agreed to revision 1 of test-term for Juju users\n",
		apiCalls: []jujutesting.StubCall{{FuncName: "SaveAgreement", Args: []interface{}{&wireformat.SaveAgreements{Agreements: []wireformat.SaveAgreement{{TermName: "test-term", TermRevision: 1}}}}}},
	}, {
		about:    "everything works with owner term",
		args:     []string{"owner/test-term/1", "--yes"},
		stdout:   "Agreed to revision 1 of owner/test-term for Juju users\n",
		apiCalls: []jujutesting.StubCall{{FuncName: "SaveAgreement", Args: []interface{}{&wireformat.SaveAgreements{Agreements: []wireformat.SaveAgreement{{TermOwner: "owner", TermName: "test-term", TermRevision: 1}}}}}},
	}, {
		about: "cannot parse revision number",
		args:  []string{"test-term/abc"},
		err:   `must specify a valid term revision "test-term/abc"`,
	}, {
		about: "missing arguments",
		args:  []string{},
		err:   "missing arguments",
	}, {
		about:  "everything works - user accepts",
		args:   []string{"test-term/1"},
		answer: "y",
		stdout: `
=== test-term/1: 0001-01-01 00:00:00 +0000 UTC ===
Test Terms
========
Do you agree to the displayed terms? (Y/n): Agreed to revision 1 of test-term for Juju users
`,
		apiCalls: []jujutesting.StubCall{{
			FuncName: "GetUnunsignedTerms", Args: []interface{}{
				&wireformat.CheckAgreementsRequest{Terms: []string{"test-term/1"}},
			},
		}, {
			FuncName: "SaveAgreement", Args: []interface{}{
				&wireformat.SaveAgreements{Agreements: []wireformat.SaveAgreement{{TermName: "test-term", TermRevision: 1}}},
			},
		}},
	}, {
		about:  "everything works - user refuses",
		args:   []string{"test-term/1"},
		answer: "n",
		stdout: `
=== test-term/1: 0001-01-01 00:00:00 +0000 UTC ===
Test Terms
========
Do you agree to the displayed terms? (Y/n): You didn't agree to the presented terms.
`,
		apiCalls: []jujutesting.StubCall{{
			FuncName: "GetUnunsignedTerms", Args: []interface{}{
				&wireformat.CheckAgreementsRequest{Terms: []string{"test-term/1"}},
			},
		}},
	}, {
		about: "must not accept 0 revision",
		args:  []string{"test-term/0", "--yes"},
		err:   `must specify a valid term revision "test-term/0"`,
	}, {
		about:  "user accepts, multiple terms",
		args:   []string{"test-term/1", "test-term/2"},
		answer: "y",
		stdout: `
=== test-term/1: 0001-01-01 00:00:00 +0000 UTC ===
Test Terms
========
Do you agree to the displayed terms? (Y/n): Agreed to revision 1 of test-term for Juju users
`,
		apiCalls: []jujutesting.StubCall{
			{
				FuncName: "GetUnunsignedTerms", Args: []interface{}{
					&wireformat.CheckAgreementsRequest{Terms: []string{"test-term/1", "test-term/2"}},
				},
			}, {
				FuncName: "SaveAgreement", Args: []interface{}{
					&wireformat.SaveAgreements{Agreements: []wireformat.SaveAgreement{
						{TermName: "test-term", TermRevision: 1},
					}},
				},
			}},
	}, {
		about: "valid then unknown arguments",
		args:  []string{"test-term/1", "unknown", "arguments"},
		err:   `must specify a valid term revision "unknown"`,
	}, {
		about: "user accepts all the terms",
		args:  []string{"test-term/1", "test-term/2", "--yes"},
		stdout: `Agreed to revision 1 of test-term for Juju users
Agreed to revision 2 of test-term for Juju users
`,
		apiCalls: []jujutesting.StubCall{
			{FuncName: "SaveAgreement", Args: []interface{}{&wireformat.SaveAgreements{
				Agreements: []wireformat.SaveAgreement{
					{TermName: "test-term", TermRevision: 1},
					{TermName: "test-term", TermRevision: 2},
				}}}}},
	}, {
		about: "everything works with term owner - user accepts",
		clientTerms: []wireformat.GetTermsResponse{{
			Name:     "test-term",
			Owner:    "test-owner",
			Revision: 1,
			Content:  testTerms,
		}},
		args:   []string{"test-owner/test-term/1"},
		answer: "y",
		stdout: `
=== test-owner/test-term/1: 0001-01-01 00:00:00 +0000 UTC ===
Test Terms
========
Do you agree to the displayed terms? (Y/n): Agreed to revision 1 of test-owner/test-term for Juju users
`,
		apiCalls: []jujutesting.StubCall{{
			FuncName: "GetUnunsignedTerms", Args: []interface{}{
				&wireformat.CheckAgreementsRequest{Terms: []string{"test-owner/test-term/1"}},
			},
		}, {
			FuncName: "SaveAgreement", Args: []interface{}{
				&wireformat.SaveAgreements{Agreements: []wireformat.SaveAgreement{{TermOwner: "test-owner", TermName: "test-term", TermRevision: 1}}},
			},
		}},
	}}
	for i, test := range tests {
		s.client.ResetCalls()
		if len(test.clientTerms) > 0 {
			s.client.setUnsignedTerms(test.clientTerms)
		}
		c.Logf("running test %d: %s", i, test.about)
		if test.answer != "" {
			answer = test.answer
		}
		ctx, err := s.runCommand(c, test.args...)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
		if ctx != nil {
			c.Assert(cmdtesting.Stdout(ctx), gc.Equals, test.stdout)
		}
		if len(test.apiCalls) > 0 {
			s.client.CheckCalls(c, test.apiCalls)
		}
	}
}

func (s *agreeSuite) runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := agree.NewAgreeCommand()
	cmd.SetClientStore(newMockStore())
	return cmdtesting.RunCommand(c, cmd, args...)
}

type mockClient struct {
	api.Client
	jujutesting.Stub

	lock          sync.Mutex
	user          string
	unsignedTerms []wireformat.GetTermsResponse
}

func (c *mockClient) setUnsignedTerms(t []wireformat.GetTermsResponse) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.unsignedTerms = t
}

// SaveAgreement saves user's agreement to the specified
// revision of the terms documents
func (c *mockClient) SaveAgreement(ctx context.Context, p *wireformat.SaveAgreements) (*wireformat.SaveAgreementResponses, error) {
	c.AddCall("SaveAgreement", p)
	responses := make([]wireformat.AgreementResponse, len(p.Agreements))
	for i, agreement := range p.Agreements {
		responses[i] = wireformat.AgreementResponse{
			User:     c.user,
			Owner:    agreement.TermOwner,
			Term:     agreement.TermName,
			Revision: agreement.TermRevision,
		}
	}
	return &wireformat.SaveAgreementResponses{responses}, nil
}

func (c *mockClient) GetUnsignedTerms(ctx context.Context, p *wireformat.CheckAgreementsRequest) ([]wireformat.GetTermsResponse, error) {
	c.MethodCall(c, "GetUnunsignedTerms", p)
	r := make([]wireformat.GetTermsResponse, len(c.unsignedTerms))
	copy(r, c.unsignedTerms)
	return r, nil
}

func (c *mockClient) GetUsersAgreements(ctx context.Context) ([]wireformat.AgreementResponse, error) {
	c.MethodCall(c, "GetUsersAgreements")
	return []wireformat.AgreementResponse{}, nil
}

func newMockStore() *jujuclient.MemStore {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}
	return store
}
