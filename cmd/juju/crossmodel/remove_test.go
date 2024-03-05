// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"bytes"
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func newRemoveCommandForTest(store jujuclient.ClientStore, api RemoveAPI) cmd.Command {
	aCmd := &removeCommand{newAPIFunc: func(controllerName string) (RemoveAPI, error) {
		return api, nil
	}}
	aCmd.SetClientStore(store)
	return modelcmd.WrapController(aCmd)
}

type removeSuite struct {
	BaseCrossModelSuite
	mockAPI *mockRemoveAPI
}

var _ = gc.Suite(&removeSuite{})

func (s *removeSuite) SetUpTest(c *gc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)
	s.mockAPI = &mockRemoveAPI{}
}

func (s *removeSuite) runRemove(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, newRemoveCommandForTest(s.store, s.mockAPI), args...)
}

func (s *removeSuite) TestNonExistentController(c *gc.C) {
	_, err := s.runRemove(c, "", "-c", "bad")
	c.Assert(err, gc.ErrorMatches, `controller bad not found`)
}

func (s *removeSuite) TestRemoveURLError(c *gc.C) {
	_, err := s.runRemove(c, "fred/model.foo/db2")
	c.Assert(err, gc.ErrorMatches, "application offer URL has invalid form.*")
}

func (s *removeSuite) TestRemoveURLWithEndpoints(c *gc.C) {
	_, err := s.runRemove(c, "fred@external/model.db2:db")
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `
These offers contain endpoints. Only specify the offer name itself.
 -fred@external/model.db2:db`[1:])
}

func (s *removeSuite) TestRemoveInconsistentControllers(c *gc.C) {
	_, err := s.runRemove(c, "ctrl:fred/model.db2", "ctrl2:fred/model.db2")
	c.Assert(err, gc.ErrorMatches, "all offer URLs must use the same controller")
}

func (s *removeSuite) TestRemoveApiError(c *gc.C) {
	s.mockAPI.msg = "fail"
	_, err := s.runRemove(c, "fred/model.db2", "-y")
	c.Assert(err, gc.ErrorMatches, ".*fail.*")
}

func (s *removeSuite) TestRemove(c *gc.C) {
	s.mockAPI.expectedURLs = []string{"fred@external/model.db2", "mary/model.db2"}
	_, err := s.runRemove(c, "fred@external/model.db2", "mary/model.db2", "-y")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *removeSuite) TestRemoveForce(c *gc.C) {
	s.mockAPI.expectedURLs = []string{"fred/model.db2", "mary/model.db2"}
	s.mockAPI.expectedForce = true
	_, err := s.runRemove(c, "fred/model.db2", "mary/model.db2", "-y", "--force")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *removeSuite) TestRemoveForceMessage(c *gc.C) {
	var stdin, stdout, stderr bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdout = &stdout
	ctx.Stderr = &stderr
	ctx.Stdin = &stdin
	stdin.WriteString("y")

	com := newRemoveCommandForTest(s.store, s.mockAPI)
	err = cmdtesting.InitCommand(com, []string{"fred/model.db2", "--force"})
	c.Assert(err, jc.ErrorIsNil)
	com.Run(ctx)

	expected := `
WARNING! This command will remove offers: fred/model.db2
This includes all relations to those offers.

Continue [y/N]? `[1:]

	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, expected)
}

func (s *removeSuite) TestRemoveNameOnly(c *gc.C) {
	s.mockAPI.expectedURLs = []string{"fred/test.db2"}
	_, err := s.runRemove(c, "db2")
	c.Assert(err, jc.ErrorIsNil)
}

type mockRemoveAPI struct {
	msg           string
	expectedForce bool
	expectedURLs  []string
}

func (s mockRemoveAPI) Close() error {
	return nil
}

func (s mockRemoveAPI) DestroyOffers(force bool, offerURLs ...string) error {
	if s.msg != "" {
		return errors.New(s.msg)
	}
	if s.expectedForce != force {
		return errors.New("mismatched force arg")
	}
	if strings.Join(s.expectedURLs, ",") != strings.Join(offerURLs, ",") {
		return errors.Errorf("mismatched URLs: %v != %v", s.expectedURLs, offerURLs)
	}
	return nil
}
