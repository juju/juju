// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	gorillaws "github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/mitchellh/go-linereader"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/apiserver/websocket"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/featureflag"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

func newEmbeddedCLIHandler(
	ctxt httpContext,
) http.Handler {
	return &embeddedCLIHandler{
		ctxt:   ctxt,
		logger: internallogger.GetLogger("juju.apiserver.embeddedcli"),
	}
}

// embeddedCLIHandler handles requests to run Juju CLi commands directly on the controller.
type embeddedCLIHandler struct {
	ctxt   httpContext
	logger corelogger.Logger
}

// ServeHTTP implements the http.Handler interface.
func (h *embeddedCLIHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	handler := func(socket *websocket.Conn) {
		h.logger.Tracef(context.TODO(), "start of *embeddedCLIHandler.ServeHTTP")
		defer socket.Close()

		// If we get to here, no more errors to report, so we report a nil
		// error.  This way the first line of the socket is always a json
		// formatted simple error.
		if sendErr := socket.SendInitialErrorV0(nil); sendErr != nil {
			h.logger.Errorf(context.TODO(), "closing websocket, %v", sendErr)
			return
		}

		// Here we configure the ping/pong handling for the websocket so
		// the server can notice when the client goes away.
		// See the long note in logsink.go for the rationale.
		_ = socket.SetReadDeadline(time.Now().Add(websocket.PongDelay))
		socket.SetPongHandler(func(string) error {
			_ = socket.SetReadDeadline(time.Now().Add(websocket.PongDelay))
			return nil
		})
		ticker := time.NewTicker(websocket.PingPeriod)
		defer ticker.Stop()

		modelUUID, valid := httpcontext.RequestModelUUID(req.Context())
		if !valid {
			h.logger.Errorf(context.TODO(), "invalid model UUID")
			return
		}
		commandCh := h.receiveCommands(socket)
		for {
			select {
			case <-h.ctxt.stop():
				return
			case <-ticker.C:
				deadline := time.Now().Add(websocket.WriteWait)
				if err := socket.WriteControl(gorillaws.PingMessage, []byte{}, deadline); err != nil {
					// This error is expected if the other end goes away. By
					// returning we close the socket through the defer call.
					h.logger.Debugf(context.TODO(), "failed to write ping: %s", err)
					return
				}
			case jujuCmd := <-commandCh:
				h.logger.Debugf(context.TODO(), "running embedded commands: %#v", jujuCmd)
				cmdErr := h.runEmbeddedCommands(req.Context(), socket, modelUUID, jujuCmd)
				// Only developers need this for debugging.
				if cmdErr != nil && featureflag.Enabled(featureflag.DeveloperMode) {
					h.logger.Debugf(context.TODO(), "command exec error: %v", cmdErr)
				}
				if err := socket.WriteJSON(params.CLICommandStatus{
					Done:  true,
					Error: apiservererrors.ServerError(cmdErr),
				}); err != nil {
					h.logger.Errorf(context.TODO(), "sending command result to caller: %v", err)
				}
			}
		}
	}
	websocket.Serve(w, req, handler)
}

func (h *embeddedCLIHandler) receiveCommands(socket *websocket.Conn) <-chan params.CLICommands {
	commandCh := make(chan params.CLICommands)

	go func() {
		for {
			var cmd params.CLICommands
			// ReadJSON() blocks until data arrives but will also be
			// unblocked when the API handler calls socket.Close as it
			// finishes.
			if err := socket.ReadJSON(&cmd); err != nil {
				// Since we don't give a list of expected error codes,
				// any CloseError type is considered unexpected.
				if gorillaws.IsUnexpectedCloseError(err) {
					h.logger.Tracef(context.TODO(), "websocket closed")
				} else {
					h.logger.Errorf(context.TODO(), "embedded CLI receive error: %v", err)
				}
				return
			}

			// Send the command.
			select {
			case <-h.ctxt.stop():
				return
			case commandCh <- cmd:
			}
		}
	}()

	return commandCh
}

func (h *embeddedCLIHandler) runEmbeddedCommands(
	ctx context.Context,
	ws *websocket.Conn,
	modelUUID string,
	commands params.CLICommands,
) error {

	// Figure out what model to run the commands on.
	resolvedModelUUID := modelUUID
	if resolvedModelUUID == "" {
		systemState, err := h.ctxt.srv.shared.statePool.SystemState()
		if err != nil {
			return errors.Trace(err)
		}
		resolvedModelUUID = systemState.ModelUUID()
	}
	modelServices, err := h.ctxt.srv.shared.domainServicesGetter.ServicesForModel(ctx, model.UUID(resolvedModelUUID))
	if err != nil {
		return errors.Trace(err)
	}
	modelInfo, err := modelServices.ModelInfo().GetModelInfo(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO (stickupkid): This is actually terrible. We should refactor
	// this out, so we can just pass an interface the handler, that hides
	// all of this nonsense.
	controllerDomainServices, err := h.ctxt.srv.shared.domainServicesGetter.ServicesForModel(ctx, h.ctxt.srv.shared.controllerModelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	controllerConfigService := controllerDomainServices.ControllerConfig()

	// Make a pipe to stream the stdout/stderr of the commands.
	errCh := make(chan error, 1)
	in, err := runCLICommands(ctx, controllerConfigService, modelInfo, errCh, commands, h.ctxt.srv.execEmbeddedCommand)
	if err != nil {
		return errors.Trace(err)
	}

	var cmdErr error
	lines := newLineReader(in)
	cmdDone := false
	outputDone := false
done:
	for {
		select {
		case <-h.ctxt.stop():
			return errors.New("command aborted due to server shutdown")
		case line, ok := <-lines.Ch:
			if !ok {
				if cmdDone {
					break done
				}
				outputDone = true
				// Wait for cmd result.
				continue
			}
			// If there's been a macaroon discharge required, we don't yet
			// process it in embedded mode so just return it so the caller
			// can deal with it, eg login again to get another macaroon.
			// This string is hard coded in the bakery library.
			idx := strings.Index(line, "cannot get discharge from")
			if idx >= 0 {
				return apiservererrors.ServerError(&apiservererrors.DischargeRequiredError{
					Cause: &bakery.DischargeRequiredError{Message: line[idx:]},
				})
			}

			if err := ws.WriteJSON(params.CLICommandStatus{
				Output: []string{line},
			}); err != nil {
				h.logger.Warningf(context.TODO(), "error writing CLI output: %v", err)
				cmdErr = err
				break done
			}
		case cmdErr = <-errCh:
			if outputDone {
				break done
			}
			// Wait for cmd output to all be read.
			cmdDone = true
		}
	}
	return cmdErr
}

// newLineReader returns a new linereader Reader for the
// provided io Reader.
func newLineReader(r io.Reader) *linereader.Reader {
	// Do the same as linereader.New(), with the juju
	// timeout values.  Changing the timeout of the
	// Reader is unsafe after calling New.
	result := &linereader.Reader{
		Reader:  r,
		Timeout: 10 * time.Millisecond,
		Ch:      make(chan string),
	}
	go result.Run()
	return result
}

// ExecEmbeddedCommandFunc defines a function which runs a named Juju command
// with the whitelisted sub commands.
type ExecEmbeddedCommandFunc func(ctx *cmd.Context, store jujuclient.ClientStore, whitelist []string, cmdPlusArgs string) int

// runCLICommands creates a CLI command instance with an in-memory copy of the controller,
// model, and account details and runs the command against the host controller.
func runCLICommands(ctx context.Context, controllerConfigService ControllerConfigService, m model.ModelInfo, errCh chan<- error, commands params.CLICommands, execEmbeddedCommand ExecEmbeddedCommandFunc) (io.Reader, error) {
	if commands.User == "" {
		return nil, errors.NotSupportedf("CLI command for anonymous user")
	}
	// Check passed in username is valid.
	if !names.IsValidUser(commands.User) {
		return nil, errors.NotValidf("user name %q", commands.User)
	}

	cfg, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Set up a juju client store used to configure the
	// embedded command to give it the controller, model
	// and account details to use.
	store := jujuclient.NewEmbeddedMemStore()
	cert, _ := cfg.CACert()
	controllerName := cfg.ControllerName()
	if controllerName == "" {
		controllerName = "interactive"
	}
	store.Controllers[controllerName] = jujuclient.ControllerDetails{
		ControllerUUID: cfg.ControllerUUID(),
		APIEndpoints:   []string{fmt.Sprintf("localhost:%d", cfg.APIPort())},
		CACert:         cert,
	}
	store.CurrentControllerName = controllerName

	qualifiedModelName := jujuclient.JoinOwnerModelName(names.NewUserTag(m.CredentialOwner.Name()), m.Name)
	store.Models[controllerName] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			qualifiedModelName: {
				ModelUUID: m.UUID.String(),
				ModelType: m.Type,
			},
		},
		CurrentModel: qualifiedModelName,
	}
	store.Accounts[controllerName] = jujuclient.AccountDetails{
		User:      commands.User,
		Password:  commands.Credentials,
		Macaroons: commands.Macaroons,
	}

	in, out := io.Pipe()
	go func() {
		defer in.Close()
		for _, cliCmd := range commands.Commands {
			ctx, err := cmd.DefaultContext()
			if err != nil {
				errCh <- errors.Trace(err)
			}
			ctx.Stdout = out
			ctx.Stderr = out
			code := execEmbeddedCommand(ctx, store, allowedEmbeddedCommands, cliCmd)
			if code != 0 {
				errCh <- errors.Annotatef(err, "command %q: exit code %d", cliCmd, code)
				continue
			}
			errCh <- nil
		}
	}()
	return in, nil
}
