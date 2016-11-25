// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// httpContext provides context for HTTP handlers.
type httpContext struct {
	// strictValidation means that empty modelUUID values are not valid.
	strictValidation bool
	// controllerModelOnly only validates the controller model.
	controllerModelOnly bool
	// srv holds the API server instance.
	srv *Server
}

// stateForRequestUnauthenticated returns a state instance appropriate for
// using for the model implicit in the given request
// without checking any authentication information.
func (ctxt *httpContext) stateForRequestUnauthenticated(r *http.Request) (*state.State, error) {
	modelUUID, err := validateModelUUID(validateArgs{
		statePool:           ctxt.srv.statePool,
		modelUUID:           r.URL.Query().Get(":modeluuid"),
		strict:              ctxt.strictValidation,
		controllerModelOnly: ctxt.controllerModelOnly,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	st, err := ctxt.srv.statePool.Get(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// stateForRequestAuthenticated returns a state instance appropriate for
// using for the model implicit in the given request.
// It also returns the authenticated entity.
func (ctxt *httpContext) stateForRequestAuthenticated(r *http.Request) (
	resultSt *state.State, resultEntity state.Entity, err error) {
	st, err := ctxt.stateForRequestUnauthenticated(r)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			ctxt.release(st)
		}
	}()

	req, err := ctxt.loginRequest(r)
	if err != nil {
		return nil, nil, errors.NewUnauthorized(err, "")
	}
	authenticator := ctxt.srv.authCtxt.authenticator(r.Host)
	entity, _, err := checkCreds(st, req, true, authenticator)
	if err != nil {
		if common.IsDischargeRequiredError(err) {
			return nil, nil, errors.Trace(err)
		}

		// Handle the special case of a worker on a controller machine
		// acting on behalf of a hosted model.
		if isMachineTag(req.AuthTag) {
			entity, err := checkControllerMachineCreds(ctxt.srv.state, req, authenticator)
			if err != nil {
				return nil, nil, errors.NewUnauthorized(err, "")
			}
			return st, entity, nil
		}

		// Any other error at this point should be treated as
		// "unauthorized".
		return nil, nil, errors.Trace(errors.NewUnauthorized(err, ""))
	}
	return st, entity, nil
}

func isMachineTag(tag string) bool {
	kind, err := names.TagKind(tag)
	return err == nil && kind == names.MachineTagKind
}

// checkPermissions verifies that given tag passes authentication check.
// For example, if only user tags are accepted, all other tags will be denied access.
func checkPermissions(tag names.Tag, acceptFunc common.GetAuthFunc) (bool, error) {
	accept, err := acceptFunc()
	if err != nil {
		return false, errors.Trace(err)
	}
	if accept(tag) {
		return true, nil
	}
	return false, errors.NotValidf("tag kind %v", tag.Kind())
}

// stateForMigration asserts that the incoming connection is from a user that
// has admin permissions on the controller model. The method also gets the
// model uuid for the model being migrated from a request header, and returns
// the state instance for that model.
func (ctxt *httpContext) stateForMigration(r *http.Request) (st *state.State, err error) {
	var user state.Entity
	st, user, err = ctxt.stateAndEntityForRequestAuthenticatedUser(r)
	if err != nil {
		return nil, err
	}
	// Pass the state pointer into the defer so the return statement doesn't
	// set the st value to nil.
	defer func(st *state.State) {
		if err != nil {
			ctxt.release(st)
		}
	}(st)

	if !st.IsController() {
		return nil, errors.BadRequestf("model is not controller model")
	}
	admin, err := st.IsControllerAdmin(user.Tag().(names.UserTag))
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !admin {
		return nil, errors.Unauthorizedf("not a controller admin")
	}

	modelUUID, err := validateModelUUID(validateArgs{
		statePool: ctxt.srv.statePool,
		modelUUID: r.Header.Get(params.MigrationModelHTTPHeader),
		strict:    true,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	migrationSt, err := ctxt.srv.statePool.Get(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := migrationSt.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if model.MigrationMode() != state.MigrationModeImporting {
		return nil, errors.BadRequestf("model not importing")
	}
	return migrationSt, nil
}

// stateForRequestAuthenticatedUser is like stateAndEntityForRequestAuthenticatedUser
// but doesn't return the entity.
func (ctxt *httpContext) stateForRequestAuthenticatedUser(r *http.Request) (*state.State, error) {
	st, _, err := ctxt.stateAndEntityForRequestAuthenticatedUser(r)
	return st, err
}

// stateAndEntityForRequestAuthenticatedUser is like stateForRequestAuthenticated
// except that it also verifies that the authenticated entity is a user.
func (ctxt *httpContext) stateAndEntityForRequestAuthenticatedUser(r *http.Request) (*state.State, state.Entity, error) {
	return ctxt.stateForRequestAuthenticatedTag(r, names.UserTagKind)
}

// stateForRequestAuthenticatedAgent is like stateForRequestAuthenticated
// except that it also verifies that the authenticated entity is an agent.
func (ctxt *httpContext) stateForRequestAuthenticatedAgent(r *http.Request) (*state.State, state.Entity, error) {
	return ctxt.stateForRequestAuthenticatedTag(r, names.MachineTagKind, names.UnitTagKind)
}

// stateForRequestAuthenticatedTag checks that the request is
// correctly authenticated, and that the authenticated entity making
// the request is of one of the specified kinds.
func (ctxt *httpContext) stateForRequestAuthenticatedTag(r *http.Request, kinds ...string) (*state.State, state.Entity, error) {
	funcs := make([]common.GetAuthFunc, len(kinds))
	for i, kind := range kinds {
		funcs[i] = common.AuthFuncForTagKind(kind)
	}
	st, entity, err := ctxt.stateForRequestAuthenticated(r)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if ok, err := checkPermissions(entity.Tag(), common.AuthAny(funcs...)); !ok {
		ctxt.release(st)
		return nil, nil, err
	}
	return st, entity, nil
}

// loginRequest forms a LoginRequest from the information
// in the given HTTP request.
func (ctxt *httpContext) loginRequest(r *http.Request) (params.LoginRequest, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		// No authorization header implies an attempt
		// to login with external user macaroon authentication.
		return params.LoginRequest{
			Macaroons: httpbakery.RequestMacaroons(r),
		}, nil
	}
	parts := strings.Fields(authHeader)
	if len(parts) != 2 || parts[0] != "Basic" {
		// Invalid header format or no header provided.
		return params.LoginRequest{}, errors.NotValidf("request format")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	// See RFC 2617, Section 2.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return params.LoginRequest{}, errors.NotValidf("request format")
	}
	tagPass := strings.SplitN(string(challenge), ":", 2)
	if len(tagPass) != 2 {
		return params.LoginRequest{}, errors.NotValidf("request format")
	}
	// Ensure that a sensible tag was passed.
	_, err = names.ParseTag(tagPass[0])
	if err != nil {
		return params.LoginRequest{}, errors.Trace(err)
	}
	return params.LoginRequest{
		AuthTag:     tagPass[0],
		Credentials: tagPass[1],
		Macaroons:   httpbakery.RequestMacaroons(r),
		Nonce:       r.Header.Get(params.MachineNonceHeader),
	}, nil
}

// release indicates that the client doesn't need this State anymore,
// so it can be removed from the pool if it needs to be.
func (ctxt *httpContext) release(st *state.State) error {
	return ctxt.srv.statePool.Release(st.ModelUUID())
}

// stop returns a channel which will be closed when a handler should
// exit.
func (ctxt *httpContext) stop() <-chan struct{} {
	return ctxt.srv.tomb.Dying()
}

// sendJSON writes a JSON-encoded response value
// to the given writer along with a trailing newline.
func sendJSON(w io.Writer, response interface{}) error {
	body, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("cannot marshal JSON result %#v: %v", response, err)
		return err
	}
	body = append(body, '\n')
	_, err = w.Write(body)
	return err
}

// sendStatusAndJSON sends an HTTP status code and
// a JSON-encoded response to a client.
func sendStatusAndJSON(w http.ResponseWriter, statusCode int, response interface{}) error {
	body, err := json.Marshal(response)
	if err != nil {
		return errors.Errorf("cannot marshal JSON result %#v: %v", response, err)
	}

	if statusCode == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	}
	w.Header().Set("Content-Type", params.ContentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(statusCode)
	if _, err := w.Write(body); err != nil {
		return errors.Annotate(err, "cannot write response")
	}
	return nil
}

// sendError sends a JSON-encoded error response
// for errors encountered during processing.
func sendError(w http.ResponseWriter, errToSend error) error {
	paramsErr, statusCode := common.ServerErrorAndStatus(errToSend)
	logger.Debugf("sending error: %d %v", statusCode, paramsErr)
	return errors.Trace(sendStatusAndJSON(w, statusCode, &params.ErrorResult{
		Error: paramsErr,
	}))
}
