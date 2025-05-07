// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/websocket/websockettest"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type embeddedCliSuite struct {
	jujutesting.ApiServerSuite
}

func (s *embeddedCliSuite) SetUpTest(c *tc.C) {
	s.WithEmbeddedCLICommand = func(ctx *cmd.Context, store jujuclient.ClientStore, whitelist []string, cmdPlusArgs string) int {
		allowed := set.NewStrings(whitelist...)
		args := strings.Split(cmdPlusArgs, " ")
		if !allowed.Contains(args[0]) {
			fmt.Fprintf(ctx.Stderr, "%q not allowed\n", args[0])
			return 1
		}
		ctrl, err := store.CurrentController()
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "%s", err.Error())
			return 1
		}
		model, err := store.CurrentModel(ctrl)
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "%s", err.Error())
			return 1
		}
		ad, err := store.AccountDetails(ctrl)
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "%s", err.Error())
			return 1
		}
		if strings.Contains(cmdPlusArgs, "macaroon error") {
			fmt.Fprintf(ctx.Stderr, "ERROR: cannot get discharge from https://controller")
			fmt.Fprintf(ctx.Stderr, "\n")
		} else {
			cmdStr := fmt.Sprintf("%s@%s:%s -> %s", ad.User, ctrl, model, cmdPlusArgs)
			fmt.Fprintf(ctx.Stdout, "%s", cmdStr)
			fmt.Fprintf(ctx.Stdout, "\n")
		}
		return 0
	}

	s.ApiServerSuite.SetUpTest(c)
}

var _ = tc.Suite(&embeddedCliSuite{})

func (s *embeddedCliSuite) TestEmbeddedCommand(c *tc.C) {
	cmdArgs := params.CLICommands{
		User:     "fred",
		Commands: []string{"status --color"},
	}
	s.assertEmbeddedCommand(c, cmdArgs, "fred@interactive:admin/controller -> status --color", nil)
}

func (s *embeddedCliSuite) TestEmbeddedCommandNotAllowed(c *tc.C) {
	cmdArgs := params.CLICommands{
		User:     "fred",
		Commands: []string{"bootstrap aws"},
	}
	s.assertEmbeddedCommand(c, cmdArgs, `"bootstrap" not allowed`, nil)
}

func (s *embeddedCliSuite) TestEmbeddedCommandMissingUser(c *tc.C) {
	cmdArgs := params.CLICommands{
		Commands: []string{"status --color"},
	}
	s.assertEmbeddedCommand(c, cmdArgs, "", &params.Error{Message: `CLI command for anonymous user not supported`, Code: "not supported"})
}

func (s *embeddedCliSuite) TestEmbeddedCommandInvalidUser(c *tc.C) {
	cmdArgs := params.CLICommands{
		User:     "123@",
		Commands: []string{"status --color"},
	}
	s.assertEmbeddedCommand(c, cmdArgs, "", &params.Error{Message: `user name "123@" not valid`, Code: params.CodeNotValid})
}

func (s *embeddedCliSuite) TestEmbeddedCommandInvalidMacaroon(c *tc.C) {
	cmdArgs := params.CLICommands{
		User:     "fred",
		Commands: []string{"status macaroon error"},
	}
	s.assertEmbeddedCommand(c, cmdArgs, "", &params.Error{
		Code:    params.CodeDischargeRequired,
		Message: `macaroon discharge required: cannot get discharge from https://controller`})
}

func (s *embeddedCliSuite) assertEmbeddedCommand(c *tc.C, cmdArgs params.CLICommands, expected string, resultErr *params.Error) {
	commandURL := s.URL(fmt.Sprintf("/model/%s/commands", s.ControllerModelUUID()), url.Values{})
	commandURL.Scheme = "wss"
	conn, _, err := dialWebsocketFromURL(c, commandURL.String(), http.Header{})
	c.Assert(err, tc.ErrorIsNil)
	defer conn.Close()

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	done := make(chan struct{})
	var result params.CLICommandStatus
	go func() {
		for {
			var update params.CLICommandStatus
			err := conn.ReadJSON(&update)
			c.Assert(err, tc.ErrorIsNil)

			result.Output = append(result.Output, update.Output...)
			result.Done = update.Done
			result.Error = update.Error
			if result.Done {
				done <- struct{}{}
				break
			}
		}
	}()

	err = conn.WriteJSON(cmdArgs)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("no command result")
	}

	// Close connection.
	err = conn.Close()
	c.Assert(err, tc.ErrorIsNil)

	var expectedOutput []string
	if expected != "" {
		expectedOutput = []string{expected}
	}
	c.Assert(result, tc.DeepEquals, params.CLICommandStatus{
		Output: expectedOutput,
		Done:   true,
		Error:  resultErr,
	})
}
