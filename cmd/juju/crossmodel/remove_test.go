// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient"
)

func newRemoveCommandForTest(store jujuclient.ClientStore, api RemoveAPI) cmd.Command {
	aCmd := &removeCommand{newAPIFunc: func(ctx context.Context, controllerName string) (RemoveAPI, error) {
		return api, nil
	}}
	aCmd.SetClientStore(store)
	return modelcmd.WrapController(aCmd)
}

type removeSuite struct {
	BaseCrossModelSuite
	mockAPI *mockRemoveAPI
}

func TestRemoveSuite(t *testing.T) {
	tc.Run(t, &removeSuite{})
}

func (s *removeSuite) SetUpTest(c *tc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)
	s.mockAPI = &mockRemoveAPI{}
}

func (s *removeSuite) runRemove(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, newRemoveCommandForTest(s.store, s.mockAPI), args...)
}

func (s *removeSuite) TestNonExistentController(c *tc.C) {
	_, err := s.runRemove(c, "", "-c", "bad")
	c.Assert(err, tc.ErrorMatches, `controller bad not found`)
}

func (s *removeSuite) TestRemoveURLError(c *tc.C) {
	_, err := s.runRemove(c, "prod/model.foo/db2")
	c.Assert(err, tc.ErrorMatches, "application offer URL has invalid form.*")
}

func (s *removeSuite) TestRemoveURLWithEndpoints(c *tc.C) {
	_, err := s.runRemove(c, "prod/model.db2:db")
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Equals, `
These offers contain endpoints. Only specify the offer name itself.
 -prod/model.db2:db`[1:])
}

func (s *removeSuite) TestRemoveInconsistentControllers(c *tc.C) {
	_, err := s.runRemove(c, "ctrl:prod/model.db2", "ctrl2:prod/model.db2")
	c.Assert(err, tc.ErrorMatches, "all offer URLs must use the same controller")
}

func (s *removeSuite) TestRemoveApiError(c *tc.C) {
	s.mockAPI.msg = "fail"
	_, err := s.runRemove(c, "prod/model.db2", "-y")
	c.Assert(err, tc.ErrorMatches, ".*fail.*")
}

func (s *removeSuite) TestRemove(c *tc.C) {
	s.mockAPI.expectedURLs = []string{"prod/model.db2", "staging/model.db2"}
	_, err := s.runRemove(c, "prod/model.db2", "staging/model.db2", "-y")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *removeSuite) TestRemoveForce(c *tc.C) {
	s.mockAPI.expectedURLs = []string{"prod/model.db2", "staging/model.db2"}
	s.mockAPI.expectedForce = true
	_, err := s.runRemove(c, "prod/model.db2", "staging/model.db2", "-y", "--force")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *removeSuite) TestRemoveForceMessage(c *tc.C) {
	var stdin, stdout, stderr bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, tc.ErrorIsNil)
	ctx.Stdout = &stdout
	ctx.Stderr = &stderr
	ctx.Stdin = &stdin
	stdin.WriteString("y")

	com := newRemoveCommandForTest(s.store, s.mockAPI)
	err = cmdtesting.InitCommand(com, []string{"prod/model.db2", "--force"})
	c.Assert(err, tc.ErrorIsNil)
	com.Run(ctx)

	expected := `
WARNING! This command will remove offers: prod/model.db2
This includes all relations to those offers.

Continue [y/N]? `[1:]

	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, expected)
}

func (s *removeSuite) TestRemoveNameOnly(c *tc.C) {
	s.mockAPI.expectedURLs = []string{"prod/test.db2"}
	_, err := s.runRemove(c, "db2")
	c.Assert(err, tc.ErrorIsNil)
}

type mockRemoveAPI struct {
	msg           string
	expectedForce bool
	expectedURLs  []string
}

func (s mockRemoveAPI) Close() error {
	return nil
}

func (s mockRemoveAPI) DestroyOffers(ctx context.Context, force bool, offerURLs ...string) error {
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
