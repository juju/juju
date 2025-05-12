// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package controlsocket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/socketlistener"
)

const (
	// jujuMetricsUserPrefix defines the "namespace" in which this worker is
	// allowed to create/remove users.
	jujuMetricsUserPrefix = "juju-metrics-"

	// userCreator is the listed "creator" of metrics users in state.
	// This user CANNOT be a local user (it must have a domain), otherwise the
	// model addUser code will complain about the user not existing.
	userCreator = "juju-metrics"
)

// AccessService is the interface for the access service.
type AccessService interface {
	// AddUser will add a new user to the database and return the UUID of the
	// user if successful. If no password is set in the incoming argument,
	// the user will be added with an activation key.
	// The following error types are possible from this function:
	//   - usererrors.UserNameNotValid: When the username supplied is not valid.
	//   - usererrors.AlreadyExists: If a user with the supplied name already exists.
	//   - usererrors.CreatorUUIDNotFound: If a creator has been supplied for the user
	//     and the creator does not exist.
	//   - auth.ErrPasswordNotValid: If the password supplied is not valid.
	AddUser(ctx context.Context, arg service.AddUserArg) (user.UUID, []byte, error)

	// GetUserByName will find and return the user associated with name. If there is no
	// user for the user name then an error that satisfies usererrors.NotFound will
	// be returned. If supplied with an invalid user name then an error that satisfies
	// usererrors.UserNameNotValid will be returned.
	//
	// GetUserByName will not return users that have been previously removed.
	GetUserByName(ctx context.Context, name user.Name) (user.User, error)

	// GetUserByAuth will find and return the user with UUID. If there is no
	// user for the name and password, then an error that satisfies
	// usererrors.NotFound will be returned. If supplied with an invalid user name
	// then an error that satisfies usererrors.UserNameNotValid will be returned.
	// It will not return users that have been previously removed.
	GetUserByAuth(ctx context.Context, name user.Name, password auth.Password) (user.User, error)

	// RemoveUser marks the user as removed and removes any credentials or
	// activation codes for the current users. Once a user is removed they are no
	// longer usable in Juju and should never be un removed.
	// The following error types are possible from this function:
	// - usererrors.UserNameNotValid: When the username supplied is not valid.
	// - usererrors.NotFound: If no user by the given UUID exists.
	RemoveUser(ctx context.Context, name user.Name) error

	// ReadUserAccessLevelForTarget returns the user access level for the
	// given user on the given target. A NotValid error is returned if the
	// subject (user) string is empty, or the target is not valid. Any errors
	// from the state layer are passed through.
	// If the access level of a user cannot be found then
	// [accesserrors.AccessNotFound] is returned.
	ReadUserAccessLevelForTarget(ctx context.Context, subject user.Name, target permission.ID) (permission.Access, error)
}

// PermissionService is the interface for the permission service.
type PermissionService interface {
	// AddUserPermission adds a user to the model with the given access.
	// If the user already has the given access, this is a no-op.
	AddUserPermission(ctx context.Context, username user.Name, access permission.Access) error
}

// Config represents configuration for the controlsocket worker.
type Config struct {
	// AccessService is the user access service for the model.
	AccessService AccessService
	// SocketName is the socket file descriptor.
	SocketName string
	// NewSocketListener is the function that creates a new socket listener.
	NewSocketListener func(socketlistener.Config) (SocketListener, error)
	// Logger is the logger used by the worker.
	Logger logger.Logger
	// ControllerModelUUID is the uuid of the controller model.
	ControllerModelUUID model.UUID
}

// Validate returns an error if config cannot drive the Worker.
func (config Config) Validate() error {
	if config.AccessService == nil {
		return errors.NotValidf("nil AccessService")
	}
	if config.ControllerModelUUID == "" {
		return errors.NotValidf("empty ControllerModelUUID")
	}
	if config.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	if config.NewSocketListener == nil {
		return errors.NotValidf("nil NewSocketListener func")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Worker is a controlsocket worker.
type Worker struct {
	catacomb catacomb.Catacomb

	accessService AccessService
	logger        logger.Logger

	controllerModelUUID model.UUID
	userCreatorName     user.Name
}

// NewWorker returns a controlsocket worker with the given config.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	userCreatorName, err := user.NewName(userCreator)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		accessService:       config.AccessService,
		logger:              config.Logger,
		controllerModelUUID: config.ControllerModelUUID,
		userCreatorName:     userCreatorName,
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
		Name: "control-socket",
		Site: &w.catacomb,
		Work: w.loop,
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

func (w *Worker) loop() error {
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
		w.writeResponse(req.Context(), resp, http.StatusBadRequest, errorf("missing request body"))
		return
	} else if err != nil {
		w.writeResponse(req.Context(), resp, http.StatusBadRequest, errorf("request body is not valid JSON: %v", err))
		return
	}

	code, err := w.addMetricsUser(req.Context(), parsedBody.Username, auth.NewPassword(parsedBody.Password))
	if err != nil {
		w.writeResponse(req.Context(), resp, code, errorf("%v", err))
		return
	}

	w.writeResponse(req.Context(), resp, code, infof("created user %q", parsedBody.Username))
}

func (w *Worker) addMetricsUser(ctx context.Context, username string, password auth.Password) (int, error) {
	validatedName, err := validateMetricsUsername(username)
	if err != nil {
		return http.StatusBadRequest, err
	}

	creatorUser, err := w.accessService.GetUserByName(ctx, w.userCreatorName)
	if err != nil {
		return http.StatusInternalServerError, errors.Annotatef(err, "retrieving creator user %q: %v", userCreator, err)
	}

	controllerModelID := permission.ID{
		ObjectType: permission.Model,
		Key:        w.controllerModelUUID.String(),
	}

	_, _, err = w.accessService.AddUser(ctx, service.AddUserArg{
		Name:        validatedName,
		DisplayName: validatedName.Name(),
		Password:    &password,
		CreatorUUID: creatorUser.UUID,
		Permission: permission.AccessSpec{
			Target: controllerModelID,
			Access: permission.ReadAccess,
		},
	})
	if errors.Is(err, usererrors.UserAlreadyExists) {
		// Retrieve existing user
		user, err := w.accessService.GetUserByAuth(ctx, validatedName, password)
		if err != nil {
			return http.StatusInternalServerError,
				fmt.Errorf("retrieving existing user %q: %v", username, err)
		}

		// We want this operation to be idempotent, but at the same time, this
		// worker shouldn't mess with users that have not been created by it.
		// So ensure the user is identical to what we would have created, and
		// otherwise error.
		if user.Disabled {
			return http.StatusForbidden, errors.Forbiddenf("user %q is disabled", user.Name)
		}
		if user.CreatorName != w.userCreatorName {
			return http.StatusConflict, errors.AlreadyExistsf("user %q (created by %q)", user.Name, user.CreatorName)
		}

		accessLevel, err := w.accessService.ReadUserAccessLevelForTarget(ctx, validatedName, controllerModelID)
		if err != nil {
			return http.StatusInternalServerError,
				fmt.Errorf("retrieving existing user %q: %v", username, err)
		} else if accessLevel != permission.ReadAccess {
			return http.StatusNotFound, fmt.Errorf(
				"unexpected permission for user %q, expected %q, got %q",
				user.Name, permission.ReadAccess, accessLevel,
			)
		}
	} else if err != nil {
		return http.StatusInternalServerError, errors.Annotatef(err, "creating user %q: %v", username, err)
	}
	return http.StatusOK, nil
}

func (w *Worker) handleRemoveMetricsUser(resp http.ResponseWriter, req *http.Request) {
	username := mux.Vars(req)["username"]
	code, err := w.removeMetricsUser(req.Context(), username)
	if err != nil {
		w.writeResponse(req.Context(), resp, code, errorf("%v", err))
		return
	}

	w.writeResponse(req.Context(), resp, code, infof("deleted user %q", username))
}

func (w *Worker) removeMetricsUser(ctx context.Context, username string) (int, error) {
	validatedName, err := validateMetricsUsername(username)
	if err != nil {
		return http.StatusBadRequest, err
	}

	// We shouldn't mess with users that weren't created by us.
	user, err := w.accessService.GetUserByName(ctx, validatedName)
	if errors.Is(err, usererrors.UserNotFound) {
		// succeed as no-op
		return http.StatusOK, nil
	} else if err != nil {
		return http.StatusInternalServerError, err
	}
	if user.CreatorName != w.userCreatorName {
		return http.StatusForbidden, errors.Forbiddenf("cannot remove user %q created by %q", user.Name, user.CreatorName)
	}

	err = w.accessService.RemoveUser(ctx, validatedName)
	// Any "not found" errors should have been caught above, so fail here.
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

func validateMetricsUsername(username string) (user.Name, error) {
	if username == "" {
		return user.Name{}, errors.BadRequestf("missing username")
	}

	if !strings.HasPrefix(username, jujuMetricsUserPrefix) {
		return user.Name{}, errors.BadRequestf("metrics username %q should have prefix %q", username, jujuMetricsUserPrefix)
	}

	name, err := user.NewName(username)
	if err != nil {
		return user.Name{}, errors.Wrap(err, errors.BadRequest)
	}

	return name, nil
}

func (w *Worker) writeResponse(ctx context.Context, resp http.ResponseWriter, statusCode int, body any) {
	w.logger.Debugf(ctx, "operation finished with HTTP status %v", statusCode)
	resp.Header().Set("Content-Type", "application/json")

	message, err := json.Marshal(body)
	if err != nil {
		w.logger.Errorf(ctx, "error marshalling response body to JSON: %v", err)
		w.logger.Errorf(ctx, "response body was %#v", body)

		// Mark this as an "internal server error"
		statusCode = http.StatusInternalServerError
		// Just write an empty response
		message = []byte("{}")
	}

	resp.WriteHeader(statusCode)
	w.logger.Tracef(ctx, "returning response %q", message)
	_, err = resp.Write(message)
	if err != nil {
		w.logger.Warningf(ctx, "error writing HTTP response: %v", err)
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
