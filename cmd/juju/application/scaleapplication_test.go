// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type ScaleApplicationSuite struct {
	testing.IsolationSuite

	mockAPI *mockScaleApplicationAPI
}

var _ = gc.Suite(&ScaleApplicationSuite{})

type mockScaleApplicationAPI struct {
	*testing.Stub
}

func (s mockScaleApplicationAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockScaleApplicationAPI) ScaleApplication(args application.ScaleApplicationParams) (params.ScaleApplicationResult, error) {
	s.MethodCall(s, "ScaleApplication", args)
	return params.ScaleApplicationResult{Info: &params.ScaleApplicationInfo{Scale: args.Scale}}, s.NextErr()
}

func (s *ScaleApplicationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockScaleApplicationAPI{Stub: &testing.Stub{}}
}

func (s *ScaleApplicationSuite) runScaleApplication(c *gc.C, args ...string) (*cmd.Context, error) {
	store := jujuclienttesting.MinimalStore()
	store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.CAAS,
		}},
	}
	return cmdtesting.RunCommand(c, NewScaleCommandForTest(s.mockAPI, store), args...)
}

func (s *ScaleApplicationSuite) TestScaleApplication(c *gc.C) {
	ctx, err := s.runScaleApplication(c, "foo", "2")
	c.Assert(err, jc.ErrorIsNil)

	stderr := cmdtesting.Stderr(ctx)
	out := strings.Replace(stderr, "\n", "", -1)
	c.Assert(out, gc.Equals, `foo scaled to 2 units`)
}

func (s *ScaleApplicationSuite) TestScaleApplicationBlocked(c *gc.C) {
	s.mockAPI.SetErrors(&params.Error{Code: params.CodeOperationBlocked, Message: "nope"})
	_, err := s.runScaleApplication(c, "foo", "2")
	c.Assert(err.Error(), jc.Contains, `could not scale application "foo": nope`)
	c.Assert(err.Error(), jc.Contains, `All operations that change model have been disabled for the current model.`)
}

func (s *ScaleApplicationSuite) TestScaleApplicationWrongModel(c *gc.C) {
	store := jujuclienttesting.MinimalStore()
	_, err := cmdtesting.RunCommand(c, NewScaleCommandForTest(s.mockAPI, store), "foo", "2")
	c.Assert(err, gc.ErrorMatches, `Juju command "scale-application" only supported on k8s container models`)
}

func (s *ScaleApplicationSuite) TestInvalidArgs(c *gc.C) {
	_, err := s.runScaleApplication(c)
	c.Assert(err, gc.ErrorMatches, `no application specified`)
	_, err = s.runScaleApplication(c, "invalid:name")
	c.Assert(err, gc.ErrorMatches, `invalid application name "invalid:name"`)
	_, err = s.runScaleApplication(c, "name")
	c.Assert(err, gc.ErrorMatches, `no scale specified`)
	_, err = s.runScaleApplication(c, "name", "scale")
	c.Assert(err, gc.ErrorMatches, `invalid scale "scale": strconv.Atoi: parsing "scale": invalid syntax`)
}
