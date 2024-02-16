// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package controlsocket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/socketlistener"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
)

const (
	// jujuMetricsUserPrefix defines the "namespace" in which this worker is
	// allowed to create/remove users.
	jujuMetricsUserPrefix = "juju-metrics-"

	// userCreator is the listed "creator" of metrics users in state.
	// This user CANNOT be a local user (it must have a domain), otherwise the
	// model addUser code will complain about the user not existing.
	userCreator = "controller@juju"
)

// Logger represents the methods used by the worker to log information.
type Logger interface {
	Errorf(string, ...any)
	Warningf(string, ...any)
	Infof(string, ...any)
	Debugf(string, ...any)
	Tracef(string, ...any)
}

// Config represents configuration for the controlsocket worker.
type Config struct {
	State             State
	Logger            Logger
	SocketName        string
	NewSocketListener func(socketlistener.Config) (SocketListener, error)
}

// Validate returns an error if config cannot drive the Worker.
func (config Config) Validate() error {
	if config.State == nil {
		return errors.NotValidf("nil State")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	if config.NewSocketListener == nil {
		return errors.NotValidf("nil NewSocketListener func")
	}
	return nil
}

// Worker is a controlsocket worker.
type Worker struct {
	config   Config
	catacomb catacomb.Catacomb
}

// NewWorker returns a controlsocket worker with the given config.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config: config,
	}
	sl, err := config.NewSocketListener(socketlistener.Config{
		Logger:           config.Logger,
		SocketName:       config.SocketName,
		RegisterHandlers: w.registerHandlers,
		ShutdownTimeout:  500 * time.Millisecond,
	})
	if err != nil {
		return nil, errors.Annotate(err, "control socket listener:")
	}

	err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.run,
		Init: []worker.Worker{sl},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) run() error {
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	}
}

func (w *Worker) registerHandlers(r *mux.Router) {
	r.HandleFunc("/metrics-users", w.handleAddMetricsUser).
		Methods(http.MethodPost)
	r.HandleFunc("/metrics-users/{username}", w.handleRemoveMetricsUser).
		Methods(http.MethodDelete)
}

type addMetricsUserBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (w *Worker) handleAddMetricsUser(resp http.ResponseWriter, req *http.Request) {
	var parsedBody addMetricsUserBody
	defer req.Body.Close()
	err := json.NewDecoder(req.Body).Decode(&parsedBody)
	if errors.Is(err, io.EOF) {
		w.writeResponse(resp, http.StatusBadRequest, errorf("missing request body"))
		return
	} else if err != nil {
		w.writeResponse(resp, http.StatusBadRequest, errorf("request body is not valid JSON: %v", err))
		return
	}

	code, err := w.addMetricsUser(parsedBody.Username, parsedBody.Password)
	if err != nil {
		w.writeResponse(resp, code, errorf("%v", err))
		return
	}

	w.writeResponse(resp, code, infof("created user %q", parsedBody.Username))
}

func (w *Worker) addMetricsUser(username, password string) (int, error) {
	err := validateMetricsUsername(username)
	if err != nil {
		return http.StatusBadRequest, err
	}

	if password == "" {
		return http.StatusBadRequest, errors.NotValidf("empty password")
	}

	user, err := w.config.State.AddUser(username, username, password, userCreator)
	cleanup := true
	// Error handling here is a bit subtle.
	switch {
	case errors.Is(err, errors.AlreadyExists):
		// Retrieve existing user
		user, err = w.config.State.User(names.NewUserTag(username))
		if err != nil {
			return http.StatusInternalServerError,
				fmt.Errorf("retrieving existing user %q: %v", username, err)
		}

		// We want this operation to be idempotent, but at the same time, this
		// worker shouldn't mess with users that have not been created by it.
		// So ensure the user is identical to what we would have created, and
		// otherwise error.
		if user.CreatedBy() != userCreator {
			return http.StatusConflict, errors.AlreadyExistsf("user %q (created by %q)", user.Name(), user.CreatedBy())
		}
		if !user.PasswordValid(password) {
			return http.StatusConflict, errors.AlreadyExistsf("user %q", user.Name())
		}

	case err == nil:
		// At this point, the operation is in a partially completed state - we've
		// added the user, but haven't granted them the correct model permissions.
		// If there is an error granting permissions, we should attempt to "rollback"
		// and remove the user again.
		defer func() {
			if cleanup == false {
				// Operation successful - nothing to clean up
				return
			}

			err := w.config.State.RemoveUser(user.UserTag())
			if err != nil {
				// Best we can do here is log an error.
				w.config.Logger.Warningf("add metrics user failed, but could not clean up user %q: %v",
					username, err)
			}
		}()

	default:
		return http.StatusInternalServerError, errors.Annotatef(err, "failed to create user %q: %v", username, err)
	}

	// Give the new user permission to access the metrics endpoint.
	var model model
	model, err = w.config.State.Model()
	if err != nil {
		return http.StatusInternalServerError, errors.Annotatef(err, "retrieving current model: %v", err)
	}

	_, err = model.AddUser(state.UserAccessSpec{
		User:      user.UserTag(),
		CreatedBy: names.NewUserTag(userCreator),
		Access:    permission.ReadAccess,
	})
	if err != nil && !errors.Is(err, errors.AlreadyExists) {
		return http.StatusInternalServerError, errors.Annotatef(err, "adding user %q to model %q: %v", username, bootstrap.ControllerModelName, err)
	}

	cleanup = false
	return http.StatusOK, nil
}

func (w *Worker) handleRemoveMetricsUser(resp http.ResponseWriter, req *http.Request) {
	username := mux.Vars(req)["username"]
	code, err := w.removeMetricsUser(username)
	if err != nil {
		w.writeResponse(resp, code, errorf("%v", err))
		return
	}

	w.writeResponse(resp, code, infof("deleted user %q", username))
}

func (w *Worker) removeMetricsUser(username string) (int, error) {
	err := validateMetricsUsername(username)
	if err != nil {
		return http.StatusBadRequest, err
	}

	userTag := names.NewUserTag(username)
	// We shouldn't mess with users that weren't created by us.
	user, err := w.config.State.User(userTag)
	if errors.Is(err, errors.NotFound) || errors.Is(err, errors.UserNotFound) || stateerrors.IsDeletedUserError(err) {
		// succeed as no-op
		return http.StatusOK, nil
	} else if err != nil {
		return http.StatusInternalServerError, err
	}
	if user.CreatedBy() != userCreator {
		return http.StatusForbidden, errors.Forbiddenf("cannot remove user %q created by %q", user.Name(), user.CreatedBy())
	}

	err = w.config.State.RemoveUser(userTag)
	// Any "not found" errors should have been caught above, so fail here.
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

func validateMetricsUsername(username string) error {
	if username == "" {
		return errors.BadRequestf("missing username")
	}

	if !names.IsValidUserName(username) {
		return errors.NotValidf("username %q", username)
	}

	if !strings.HasPrefix(username, jujuMetricsUserPrefix) {
		return errors.BadRequestf("metrics username %q should have prefix %q", username, jujuMetricsUserPrefix)
	}

	return nil
}

func (w *Worker) writeResponse(resp http.ResponseWriter, statusCode int, body any) {
	w.config.Logger.Debugf("operation finished with HTTP status %v", statusCode)
	resp.Header().Set("Content-Type", "application/json")

	message, err := json.Marshal(body)
	if err != nil {
		w.config.Logger.Errorf("error marshalling response body to JSON: %v", err)
		w.config.Logger.Errorf("response body was %#v", body)

		// Mark this as an "internal server error"
		statusCode = http.StatusInternalServerError
		// Just write an empty response
		message = []byte("{}")
	}

	resp.WriteHeader(statusCode)
	w.config.Logger.Tracef("returning response %q", message)
	_, err = resp.Write(message)
	if err != nil {
		w.config.Logger.Warningf("error writing HTTP response: %v", err)
	}
}

// infof returns an informational response body that can be marshalled into
// JSON (in the case of a successful operation). It has the form
//
//	{"message": <provided info message>}
func infof(format string, args ...any) any {
	return struct {
		Message string `json:"message"`
	}{
		Message: fmt.Sprintf(format, args...),
	}
}

// errorf returns an error response body that can be marshalled into JSON (in
// the case of a failed operation). It has the form
//
//	{"error": <provided error message>}
func errorf(format string, args ...any) any {
	return struct {
		Error string `json:"error"`
	}{
		Error: fmt.Sprintf(format, args...),
	}
}
