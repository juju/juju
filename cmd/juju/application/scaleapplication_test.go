// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type ScaleApplicationSuite struct {
	testhelpers.IsolationSuite

	mockAPI *mockScaleApplicationAPI
}

func TestScaleApplicationSuite(t *stdtesting.T) { tc.Run(t, &ScaleApplicationSuite{}) }

type mockScaleApplicationAPI struct {
	*testhelpers.Stub
}

func (s mockScaleApplicationAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockScaleApplicationAPI) ScaleApplication(ctx context.Context, args application.ScaleApplicationParams) (params.ScaleApplicationResult, error) {
	s.MethodCall(s, "ScaleApplication", args)
	return params.ScaleApplicationResult{Info: &params.ScaleApplicationInfo{Scale: args.Scale}}, s.NextErr()
}

func (s *ScaleApplicationSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockScaleApplicationAPI{Stub: &testhelpers.Stub{}}
}

func (s *ScaleApplicationSuite) runScaleApplication(c *tc.C, args ...string) (*cmd.Context, error) {
	store := jujuclienttesting.MinimalStore()
	store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.CAAS,
		}},
	}
	return cmdtesting.RunCommand(c, NewScaleCommandForTest(s.mockAPI, store), args...)
}

func (s *ScaleApplicationSuite) TestScaleApplication(c *tc.C) {
	ctx, err := s.runScaleApplication(c, "foo", "2")
	c.Assert(err, tc.ErrorIsNil)

	stderr := cmdtesting.Stderr(ctx)
	out := strings.Replace(stderr, "\n", "", -1)
	c.Assert(out, tc.Equals, `foo scaled to 2 units`)
}

func (s *ScaleApplicationSuite) TestScaleApplicationBlocked(c *tc.C) {
	s.mockAPI.SetErrors(&params.Error{Code: params.CodeOperationBlocked, Message: "nope"})
	_, err := s.runScaleApplication(c, "foo", "2")
	c.Assert(err.Error(), tc.Contains, `could not scale application "foo": nope`)
	c.Assert(err.Error(), tc.Contains, `All operations that change model have been disabled for the current model.`)
}

func (s *ScaleApplicationSuite) TestScaleApplicationWrongModel(c *tc.C) {
	store := jujuclienttesting.MinimalStore()
	_, err := cmdtesting.RunCommand(c, NewScaleCommandForTest(s.mockAPI, store), "foo", "2")
	c.Assert(err, tc.ErrorMatches, `Juju command "scale-application" only supported on k8s container models`)
}

func (s *ScaleApplicationSuite) TestInvalidArgs(c *tc.C) {
	_, err := s.runScaleApplication(c)
	c.Assert(err, tc.ErrorMatches, `no application specified`)
	_, err = s.runScaleApplication(c, "invalid:name")
	c.Assert(err, tc.ErrorMatches, `invalid application name "invalid:name"`)
	_, err = s.runScaleApplication(c, "name")
	c.Assert(err, tc.ErrorMatches, `no scale specified`)
	_, err = s.runScaleApplication(c, "name", "scale")
	c.Assert(err, tc.ErrorMatches, `invalid scale "scale": strconv.Atoi: parsing "scale": invalid syntax`)
}
