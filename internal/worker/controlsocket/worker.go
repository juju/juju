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
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/service"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	"github.com/juju/juju/internal/auth"
	internalerrors "github.com/juju/juju/internal/errors"
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

	// maxPayloadBytes is the maximum size of any payload.
	maxPayloadBytes = 1 << 20 // 1MiB
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

// TracingService is the interface for the tracing service.
type TracingService interface {
	// SetCharmTracingConfig sets the charm tracing configuration to the provided values.
	SetCharmTracingConfig(ctx context.Context, config tracingservice.CharmTracingConfig) error
}

// Config represents configuration for the controlsocket worker.
type Config struct {
	// AccessService is the user access service for the model.
	AccessService AccessService
	// TracingService is the tracing service for the model.
	TracingService TracingService
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
		return internalerrors.New("nil AccessService").Add(coreerrors.NotValid)
	}
	if config.ControllerModelUUID == "" {
		return internalerrors.New("empty ControllerModelUUID").Add(coreerrors.NotValid)
	}
	if config.SocketName == "" {
		return internalerrors.New("empty SocketName").Add(coreerrors.NotValid)
	}
	if config.NewSocketListener == nil {
		return internalerrors.New("nil NewSocketListener func").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return internalerrors.New("nil Logger").Add(coreerrors.NotValid)
	}
	return nil
}

// Worker is a controlsocket worker.
type Worker struct {
	catacomb catacomb.Catacomb

	accessService  AccessService
	tracingService TracingService

	controllerModelUUID model.UUID
	userCreatorName     user.Name

	logger logger.Logger
}

// NewWorker returns a controlsocket worker with the given config.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, internalerrors.Capture(err)
	}

	userCreatorName, err := user.NewName(userCreator)
	if err != nil {
		return nil, internalerrors.Errorf("invalid user creator name %q: %w", userCreator, err)
	}

	w := &Worker{
		accessService:       config.AccessService,
		tracingService:      config.TracingService,
		controllerModelUUID: config.ControllerModelUUID,
		userCreatorName:     userCreatorName,

		logger: config.Logger,
	}

	sl, err := config.NewSocketListener(socketlistener.Config{
		Logger:           config.Logger,
		SocketName:       config.SocketName,
		RegisterHandlers: w.registerHandlers,
		ShutdownTimeout:  500 * time.Millisecond,
	})
	if err != nil {
		return nil, internalerrors.Errorf("control socket listener: %w", err)
	}

	err = catacomb.Invoke(catacomb.Plan{
		Name: "control-socket",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{sl},
	})
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	return w, nil
}

// Kill stops the worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the worker to stop and returns any error that caused it to
// stop.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {
	<-w.catacomb.Dying()
	return w.catacomb.ErrDying()
}

func (w *Worker) registerHandlers(r *mux.Router) {
	// metrics-users endpoint for managing users that can access metrics. This
	// is a POST endpoint that accepts a JSON body with the following format:
	//
	// {
	//   "username": <string>,
	//   "password": <string>,
	// }
	//
	// The username must have the prefix "juju-metrics-", and the worker will
	// create a user with read access to the controller model. If a user with
	// the given name already exists, it will be reused if it has the expected
	// permissions and was created by this worker, and otherwise an error will
	// be returned.
	//
	// A user created by this endpoint can be removed by sending a DELETE
	// request to the /metrics-users/{username} endpoint.
	r.Handle("/metrics-users", w.handleJSONPost(w.handleAddMetricsUser)).
		Methods(http.MethodPost)
	r.HandleFunc("/metrics-users/{username}", w.handleRemoveMetricsUser).
		Methods(http.MethodDelete)

	// charm-tracing-config endpoint for managing charm tracing configuration.
	// This is a POST endpoint that accepts a JSON body with the following
	// format:
	//
	// {
	//   "http_endpoint": <string>,
	//   "grpc_endpoint": <string>,
	//   "ca_cert": <string>,
	// }
	//
	// The worker will update the charm tracing configuration with the provided
	// values. Any field that are omitted or empty will be removed from the
	// charm tracing configuration.
	r.Handle("/charm-tracing-config", w.handleJSONPost(w.handleSetCharmTracingConfig)).
		Methods(http.MethodPost)
}

type addMetricsUserBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (w *Worker) handleAddMetricsUser(resp http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var parsedBody addMetricsUserBody
	if err := json.NewDecoder(req.Body).Decode(&parsedBody); err != nil {
		var maxBytesErr *http.MaxBytesError
		switch {
		case internalerrors.Is(err, io.EOF):
			w.writeErrorResponse(ctx, resp, http.StatusBadRequest,
				internalerrors.New("missing request body"))
		case internalerrors.As(err, &maxBytesErr):
			w.writeErrorResponse(ctx, resp, http.StatusRequestEntityTooLarge,
				internalerrors.Errorf("request body must not exceed %d bytes", maxPayloadBytes))
		default:
			w.writeErrorResponse(ctx, resp, http.StatusBadRequest,
				internalerrors.Errorf("request body is not valid JSON: %v", err))
		}
		return
	}

	code, err := w.addMetricsUser(ctx, parsedBody.Username, auth.NewPassword(parsedBody.Password))
	if err != nil {
		w.writeErrorResponse(ctx, resp, code, err)
		return
	}

	w.writeResponse(ctx, resp, code, infof("created user %q", parsedBody.Username))
}

func (w *Worker) addMetricsUser(ctx context.Context, username string, password auth.Password) (int, error) {
	validatedName, err := validateMetricsUsername(username)
	if err != nil {
		return http.StatusBadRequest, err
	}

	creatorUser, err := w.accessService.GetUserByName(ctx, w.userCreatorName)
	if err != nil {
		return http.StatusInternalServerError,
			internalerrors.Errorf("retrieving creator user %q: %w", userCreator, err)
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
	if internalerrors.Is(err, usererrors.UserAlreadyExists) {
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
			return http.StatusForbidden, internalerrors.Errorf("user %q is disabled", user.Name).
				Add(coreerrors.Forbidden)
		}
		if user.CreatorName != w.userCreatorName {
			return http.StatusConflict, internalerrors.Errorf("user %q (created by %q)", user.Name, user.CreatorName).
				Add(coreerrors.AlreadyExists)
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
		return http.StatusInternalServerError, internalerrors.Errorf("creating user %q: %w", username, err)
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
	if internalerrors.Is(err, usererrors.UserNotFound) {
		// succeed as no-op
		return http.StatusOK, nil
	} else if err != nil {
		return http.StatusInternalServerError, err
	}
	if user.CreatorName != w.userCreatorName {
		return http.StatusForbidden, internalerrors.Errorf("cannot remove user %q created by %q", user.Name, user.CreatorName).
			Add(coreerrors.Forbidden)
	}

	err = w.accessService.RemoveUser(ctx, validatedName)
	// Any "not found" errors should have been caught above, so fail here.
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

type setCharmTracingConfig struct {
	HTTPEndpoint string `json:"http_endpoint"`
	GRPCEndpoint string `json:"grpc_endpoint"`
	CACert       string `json:"ca_cert"`
}

func (w *Worker) handleSetCharmTracingConfig(resp http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var parsedBody setCharmTracingConfig
	if err := json.NewDecoder(req.Body).Decode(&parsedBody); err != nil {
		var maxBytesErr *http.MaxBytesError
		switch {
		case internalerrors.Is(err, io.EOF):
			w.writeErrorResponse(ctx, resp, http.StatusBadRequest,
				internalerrors.New("missing request body"))
		case internalerrors.As(err, &maxBytesErr):
			w.writeErrorResponse(ctx, resp, http.StatusRequestEntityTooLarge,
				internalerrors.Errorf("request body must not exceed %d bytes", maxPayloadBytes))
		default:
			w.writeErrorResponse(ctx, resp, http.StatusBadRequest,
				internalerrors.Errorf("request body is not valid JSON: %v", err))
		}
		return
	}

	err := w.tracingService.SetCharmTracingConfig(ctx, tracingservice.CharmTracingConfig{
		HTTPEndpoint:  parsedBody.HTTPEndpoint,
		GRPCEndpoint:  parsedBody.GRPCEndpoint,
		CACertificate: parsedBody.CACert,
	})
	if err != nil {
		w.writeErrorResponse(ctx, resp, http.StatusInternalServerError, err)
		return
	}

	w.writeResponse(ctx, resp, http.StatusOK, "updated charm tracing config")
}

func (w *Worker) handleJSONPost(fn func(http.ResponseWriter, *http.Request)) http.Handler {
	errorWriter := errorResponseWriter(w.writeErrorResponse)

	return closeRequestBodyMiddleware(
		contentTypeMiddleware(
			contentLengthMiddleware(
				http.HandlerFunc(fn),
				errorWriter,
			),
			errorWriter,
		),
	)
}

func (w *Worker) writeErrorResponse(ctx context.Context, resp http.ResponseWriter, statusCode int, err error) {
	w.logger.Errorf(ctx, "failed to handle request: %v", err)

	w.writeResponse(ctx, resp, statusCode, errorf("%s", err.Error()))
}

func (w *Worker) writeResponse(ctx context.Context, resp http.ResponseWriter, statusCode int, body any) {
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

func validateMetricsUsername(username string) (user.Name, error) {
	if username == "" {
		return user.Name{}, internalerrors.Errorf("missing username").Add(coreerrors.BadRequest)
	}

	if !strings.HasPrefix(username, jujuMetricsUserPrefix) {
		return user.Name{}, internalerrors.Errorf("metrics username %q should have prefix %q", username, jujuMetricsUserPrefix).
			Add(coreerrors.BadRequest)
	}

	name, err := user.NewName(username)
	if err != nil {
		return user.Name{}, errors.Wrap(err, errors.BadRequest)
	}

	return name, nil
}
