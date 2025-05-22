// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

func newOfferCommandForTest(
	store jujuclient.ClientStore,
	api OfferAPI,
) cmd.Command {
	aCmd := &offerCommand{
		newAPIFunc: func(ctx context.Context) (OfferAPI, error) {
			return api, nil
		},
		refreshModels: func(context.Context, jujuclient.ClientStore, string) error {
			return nil
		},
	}
	aCmd.SetClientStore(store)
	return modelcmd.WrapController(aCmd)
}

type offerSuite struct {
	BaseCrossModelSuite
	mockAPI *mockOfferAPI
	args    []string
}

func TestOfferSuite(t *testing.T) {
	tc.Run(t, &offerSuite{})
}

func (s *offerSuite) SetUpTest(c *tc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)

	s.mockAPI = newMockOfferAPI()
	s.args = nil
}

func (s *offerSuite) TestOfferNoArgs(c *tc.C) {
	s.assertOfferErrorOutput(c, ".*an offer must at least specify application endpoint.*")
}

func (s *offerSuite) TestOfferTooManyArgs(c *tc.C) {
	s.args = []string{"tst:db", "alias", "extra"}
	s.assertOfferErrorOutput(c, `unrecognized args: \["extra"\]`)
}

func (s *offerSuite) TestOfferInvalidApplication(c *tc.C) {
	s.args = []string{"123:"}
	s.assertOfferErrorOutput(c, `.*application name "123" not valid.*`)
}

func (s *offerSuite) TestOfferInvalidModel(c *tc.C) {
	s.args = []string{"$model.123:db"}
	s.assertOfferErrorOutput(c, `.*model name "\$model" not valid.*`)
}

func (s *offerSuite) TestOfferNoCurrentModel(c *tc.C) {
	s.store.Models["test-master"].CurrentModel = ""
	s.args = []string{"app:db"}
	s.assertOfferErrorOutput(c, `no current model, use juju switch to select a model on which to operate`)
}

func (s *offerSuite) TestOfferInvalidEndpoints(c *tc.C) {
	s.args = []string{"tst/123"}
	s.assertOfferErrorOutput(c, `.*endpoints must conform to format.*`)
}

func (s *offerSuite) TestOfferNoEndpoints(c *tc.C) {
	s.args = []string{"tst:"}
	s.assertOfferErrorOutput(c, `.*specify endpoints for tst.*`)
}

func (s *offerSuite) assertOfferErrorOutput(c *tc.C, expected string) {
	_, err := s.runOffer(c, s.args...)
	c.Assert(errors.Cause(err), tc.ErrorMatches, expected)
}

func (s *offerSuite) runOffer(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, newOfferCommandForTest(s.store, s.mockAPI), args...)
}

func (s *offerSuite) TestOfferCallErred(c *tc.C) {
	s.args = []string{"tst:db"}
	s.mockAPI.errCall = true
	s.assertOfferErrorOutput(c, ".*aborted.*")
}

func (s *offerSuite) TestOfferDataErred(c *tc.C) {
	s.args = []string{"tst:db"}
	s.mockAPI.errData = true
	s.assertOfferErrorOutput(c, ".*failed.*")
}

func (s *offerSuite) TestOfferValid(c *tc.C) {
	s.args = []string{"tst:db"}
	s.assertOfferOutput(c, "test", "tst", "tst", []string{"db"})
	c.Assert(s.mockAPI.modelUUID, tc.Equals, "fred-uuid")
}

func (s *offerSuite) TestOfferWithAlias(c *tc.C) {
	s.args = []string{"tst:db", "hosted-tst"}
	s.assertOfferOutput(c, "test", "hosted-tst", "tst", []string{"db"})
}

func (s *offerSuite) TestOfferExplicitModel(c *tc.C) {
	s.args = []string{"bob/prod.tst:db"}
	s.assertOfferOutput(c, "prod", "tst", "tst", []string{"db"})
}

func (s *offerSuite) TestOfferMultipleEndpoints(c *tc.C) {
	s.args = []string{"tst:db,admin"}
	s.assertOfferOutput(c, "test", "tst", "tst", []string{"db", "admin"})
}

func (s *offerSuite) assertOfferOutput(c *tc.C, expectedModel, expectedOffer, expectedApplication string, endpoints []string) {
	_, err := s.runOffer(c, s.args...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mockAPI.offers[expectedOffer], tc.SameContents, endpoints)
}

type mockOfferAPI struct {
	errCall, errData bool
	modelUUID        string
	offers           map[string][]string
	applications     map[string]string
	descs            map[string]string
}

func newMockOfferAPI() *mockOfferAPI {
	mock := &mockOfferAPI{}
	mock.offers = make(map[string][]string)
	mock.descs = make(map[string]string)
	mock.applications = make(map[string]string)
	return mock
}

func (s *mockOfferAPI) Close() error {
	return nil
}

func (s *mockOfferAPI) Offer(ctx context.Context, modelUUID, application string, endpoints []string, owner, offerName, desc string) ([]params.ErrorResult, error) {
	if s.errCall {
		return nil, errors.New("aborted")
	}
	result := make([]params.ErrorResult, 1)
	if s.errData {
		result[0].Error = apiservererrors.ServerError(errors.New("failed"))
		return result, nil
	}
	if owner != "bob" {
		return nil, errors.Errorf("unexpected offer owner %q", owner)
	}
	s.modelUUID = modelUUID
	if offerName == "" {
		offerName = application
	}
	s.offers[offerName] = endpoints
	s.applications[offerName] = application
	s.descs[offerName] = desc
	return result, nil
}
