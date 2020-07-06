// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiapplication "github.com/juju/juju/api/application"
	commonerrors "github.com/juju/juju/apiserver/common/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&RemoveApplicationCmdSuite{})

type RemoveApplicationCmdSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api *testApplicationRemoveUnitAPI

	apiFunc func() (application.RemoveApplicationAPI, int, error)
	store   *jujuclient.MemStore
}

func (s *RemoveApplicationCmdSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.api = &testApplicationRemoveUnitAPI{
		Stub: &jujutesting.Stub{},
	}
	s.store = jujuclienttesting.MinimalStore()
	s.apiFunc = func() (application.RemoveApplicationAPI, int, error) {
		return s.api, 5, nil
	}
}

func (s *RemoveApplicationCmdSuite) TestForceFlagUnset(c *gc.C) {
	s.assertAPIForceFlag(c, []string{"real-app"}, false)
}

func (s *RemoveApplicationCmdSuite) TestForceFlagSet(c *gc.C) {
	s.assertAPIForceFlag(c, []string{"real-app", "--force"}, true)
}

func (s *RemoveApplicationCmdSuite) runRemoveApplication(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, application.NewRemoveApplicationCommandForTest(s.apiFunc, s.store), args...)
}

func (s *RemoveApplicationCmdSuite) assertAPIForceFlag(c *gc.C, args []string, expectedValue bool) {
	s.api.destroyApplications = func(args apiapplication.DestroyApplicationsParams) ([]params.DestroyApplicationResult, error) {
		c.Assert(args.Force, gc.Equals, expectedValue)
		return []params.DestroyApplicationResult{
			{Info: &params.DestroyApplicationInfo{}},
		}, nil
	}
	ctx, err := s.runRemoveApplication(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "removing application real-app\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	s.api.CheckCallNames(c, "DestroyApplications", "Close")
}

func (s *RemoveApplicationCmdSuite) setupRace(c *gc.C, raceyApplications []string) {
	s.api.destroyApplications = func(args apiapplication.DestroyApplicationsParams) ([]params.DestroyApplicationResult, error) {
		results := make([]params.DestroyApplicationResult, len(args.Applications))
		for i, app := range args.Applications {
			results[i].Info = &params.DestroyApplicationInfo{}
			for _, poison := range raceyApplications {
				if app == poison {
					err := errors.NewNotSupported(nil, "change detected")
					results[i].Error = commonerrors.ServerError(err)
				}
			}
		}
		return results, nil
	}
}

func (s *RemoveApplicationCmdSuite) TestHandlingNotSupportedDoesNotAffectBaseCase(c *gc.C) {
	s.setupRace(c, []string{"do-not-remove"})

	ctx, err := s.runRemoveApplication(c, []string{"real-app"}...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "removing application real-app\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	s.api.CheckCallNames(c, "DestroyApplications", "Close")
}

func (s *RemoveApplicationCmdSuite) TestHandlingNotSupported(c *gc.C) {
	s.setupRace(c, []string{"do-not-remove"})

	ctx, err := s.runRemoveApplication(c, []string{"do-not-remove"}...)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, ""+
		"removing application do-not-remove failed: "+
		"another user was updating application; please try again\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	s.api.CheckCallNames(c, "DestroyApplications", "Close")
}

func (s *RemoveApplicationCmdSuite) TestHandlingNotSupportedMultipleApps(c *gc.C) {
	s.setupRace(c, []string{"do-not-remove"})

	ctx, err := s.runRemoveApplication(c, []string{"real-app", "do-not-remove", "another"}...)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, ""+
		"removing application real-app\n"+
		"removing application do-not-remove failed: "+
		"another user was updating application; please try again\n"+
		"removing application another\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	s.api.CheckCallNames(c, "DestroyApplications", "Close")
}

type testApplicationRemoveUnitAPI struct {
	*jujutesting.Stub

	destroyApplications func(args apiapplication.DestroyApplicationsParams) ([]params.DestroyApplicationResult, error)

	destroyUnits func(args apiapplication.DestroyUnitsParams) ([]params.DestroyUnitResult, error)
}

func (a *testApplicationRemoveUnitAPI) DestroyApplications(args apiapplication.DestroyApplicationsParams) ([]params.DestroyApplicationResult, error) {
	a.AddCall("DestroyApplications", args)
	return a.destroyApplications(args)
}

func (a *testApplicationRemoveUnitAPI) DestroyUnits(args apiapplication.DestroyUnitsParams) ([]params.DestroyUnitResult, error) {
	a.AddCall("DestroyUnits", args)
	return a.destroyUnits(args)
}

func (a *testApplicationRemoveUnitAPI) Close() error {
	a.AddCall("Close")
	return a.NextErr()
}

func (a *testApplicationRemoveUnitAPI) BestAPIVersion() int {
	panic("BestAPIVersion not implemented here")
}

func (a *testApplicationRemoveUnitAPI) ModelUUID() string {
	panic("ModelUUID not implemented here")
}

func (a *testApplicationRemoveUnitAPI) ScaleApplication(ps apiapplication.ScaleApplicationParams) (params.ScaleApplicationResult, error) {
	panic("ScaleApplication not implemented here")
}

func (a *testApplicationRemoveUnitAPI) DestroyDeprecated(appName string) error {
	panic("DestroyDeprecated not implemented here")
}

func (a *testApplicationRemoveUnitAPI) DestroyUnitsDeprecated(unitNames ...string) error {
	panic("DestroyUnitsDeprecated not implemented here")
}
