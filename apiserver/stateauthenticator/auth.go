// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// AgentTags are those used by any Juju agent.
var AgentTags = []string{
	names.MachineTagKind,
	names.ControllerAgentTagKind,
	names.UnitTagKind,
	names.ApplicationTagKind,
	names.ModelTagKind,
}

// Authenticator is an implementation of httpcontext.Authenticator,
// using *state.State for authentication.
//
// This Authenticator only works with requests that have been handled
// by one of the httpcontext.*ModelHandler handlers.
type Authenticator struct {
	statePool               *state.StatePool
	controllerConfigService ControllerConfigService
	authContext             *authContext
}

// PermissionDelegator implements authentication.PermissionDelegator
type PermissionDelegator struct {
	State *state.State
}

// ControllerConfigService is an interface that can be implemented by
// types that can return a controller config.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// NewAuthenticator returns a new Authenticator using the given StatePool.
func NewAuthenticator(
	statePool *state.StatePool,
	systemState *state.State,
	controllerConfigService ControllerConfigService,
	userService UserService,
	agentAuthFactory AgentAuthenticatorFactory,
	clock clock.Clock,
) (*Authenticator, error) {
	authContext, err := newAuthContext(systemState, controllerConfigService, userService, agentAuthFactory, clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Authenticator{
		statePool:               statePool,
		controllerConfigService: controllerConfigService,
		authContext:             authContext,
	}, nil
}

// Maintain periodically expires local login interactions.
func (a *Authenticator) Maintain(done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-a.authContext.clock.After(authentication.LocalLoginInteractionTimeout):
			now := a.authContext.clock.Now()
			a.authContext.localUserInteractions.Expire(now)
		}
	}
}

// CreateLocalLoginMacaroon is part of the
// httpcontext.LocalMacaroonAuthenticator interface.
func (a *Authenticator) CreateLocalLoginMacaroon(ctx context.Context, tag names.UserTag, version bakery.Version) (*macaroon.Macaroon, error) {
	mac, err := a.authContext.CreateLocalLoginMacaroon(ctx, tag, version)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return mac.M(), nil
}

// AddHandlers adds the handlers to the given mux for handling local
// macaroon logins.
func (a *Authenticator) AddHandlers(mux *apiserverhttp.Mux) error {
	h := &localLoginHandlers{
		authCtxt:   a.authContext,
		userTokens: map[string]string{},
	}
	h.AddHandlers(mux)
	return nil
}

// Authenticate is part of the httpcontext.Authenticator interface.
func (a *Authenticator) Authenticate(req *http.Request) (authentication.AuthInfo, error) {
	modelUUID := httpcontext.RequestModelUUID(req)
	if modelUUID == "" {
		return authentication.AuthInfo{}, errors.New("model UUID not found")
	}
	loginRequest, err := LoginRequest(req)
	if err != nil {
		return authentication.AuthInfo{}, errors.Trace(err)
	}
	authParams := authentication.AuthParams{
		Credentials:   loginRequest.Credentials,
		Nonce:         loginRequest.Nonce,
		Macaroons:     loginRequest.Macaroons,
		BakeryVersion: loginRequest.BakeryVersion,
	}
	if loginRequest.AuthTag != "" {
		authParams.AuthTag, err = names.ParseTag(loginRequest.AuthTag)
		if err != nil {
			return authentication.AuthInfo{}, errors.Trace(err)
		}
	}
	return a.AuthenticateLoginRequest(req.Context(), req.Host, modelUUID, authParams)
}

// AuthenticateLoginRequest authenticates a LoginRequest.
func (a *Authenticator) AuthenticateLoginRequest(
	ctx context.Context,
	serverHost string,
	modelUUID string,
	authParams authentication.AuthParams,
) (_ authentication.AuthInfo, err error) {
	defer func() {
		if errors.Is(err, apiservererrors.ErrNoCreds) {
			err = errors.NewNotSupported(err, "")
		}
	}()

	st, err := a.statePool.Get(modelUUID)
	if err != nil {
		return authentication.AuthInfo{}, errors.Trace(err)
	}
	defer st.Release()

	authenticator := a.authContext.authenticatorForState(serverHost, st.State)
	authInfo, err := a.checkCreds(ctx, st.State, authParams, authenticator)
	if err == nil {
		return authInfo, nil
	}

	var dischargeRequired *apiservererrors.DischargeRequiredError
	if errors.As(err, &dischargeRequired) || errors.Is(err, errors.NotProvisioned) {
		// TODO(axw) move out of common?
		return authentication.AuthInfo{}, errors.Trace(err)
	}

	systemState, errS := a.statePool.SystemState()
	if errS != nil {
		return authentication.AuthInfo{}, errors.Trace(err)
	}

	_, isMachineTag := authParams.AuthTag.(names.MachineTag)
	_, isControllerAgentTag := authParams.AuthTag.(names.ControllerAgentTag)
	if (isMachineTag || isControllerAgentTag) && !st.IsController() {
		// Controller agents are allowed to log into any model.
		authenticator := a.authContext.authenticator(serverHost)
		var err2 error
		authInfo, err2 = a.checkCreds(
			ctx,
			systemState,
			authParams, authenticator,
		)
		if err2 == nil && authInfo.Controller {
			err = nil
		}
	}
	if err != nil {
		return authentication.AuthInfo{}, errors.NewUnauthorized(err, "")
	}

	authInfo.Delegator = &PermissionDelegator{State: systemState}
	return authInfo, nil
}

// SubjectPermissions implements PermissionDelegator
func (p *PermissionDelegator) SubjectPermissions(
	e authentication.Entity,
	s names.Tag,
) (permission.Access, error) {
	userTag, ok := e.Tag().(names.UserTag)
	if !ok {
		return permission.NoAccess, errors.Errorf("%s is not a user", names.ReadableString(e.Tag()))
	}

	return p.State.UserPermission(userTag, s)
}

func (p *PermissionDelegator) PermissionError(
	_ names.Tag,
	_ permission.Access,
) error {
	return apiservererrors.ErrPerm
}

func (a *Authenticator) checkCreds(
	ctx context.Context,
	st *state.State,
	authParams authentication.AuthParams,
	authenticator authentication.EntityAuthenticator,
) (authentication.AuthInfo, error) {
	entity, err := authenticator.Authenticate(ctx, authParams)
	if err != nil {
		return authentication.AuthInfo{}, errors.Trace(err)
	}

	authInfo := authentication.AuthInfo{
		Delegator: &PermissionDelegator{State: st},
		Entity:    entity,
	}

	switch entity.Tag().Kind() {
	case names.UserTagKind:
		// TODO (stickupkid): This is incorrect. We should only be updating the
		// last login time if they've been authorized (not just authenticated).
		// For now we'll leave it as is, but we should fix this.
		userTag := entity.Tag().(names.UserTag)

		st := a.authContext.st
		model, err := st.Model()
		if err != nil {
			return authentication.AuthInfo{}, errors.Trace(err)
		}
		modelAccess, err := st.UserAccess(userTag, model.ModelTag())
		if err != nil && !errors.Is(err, errors.NotFound) {
			return authentication.AuthInfo{}, errors.Trace(err)
		}

		// This is permission checking at the wrong level, but we can keep it
		// here for now.
		if err := a.checkPerms(ctx, modelAccess, userTag); err != nil {
			return authentication.AuthInfo{}, errors.Trace(err)
		}

		if err := a.updateUserLastLogin(ctx, modelAccess, userTag, model); err != nil {
			return authentication.AuthInfo{}, errors.Trace(err)
		}

	case names.MachineTagKind, names.ControllerAgentTagKind:
		// Currently only machines and controller agents are managers in the
		// context of a controller.
		authInfo.Controller = a.isManager(entity)
	}

	return authInfo, nil
}

func (a *Authenticator) checkPerms(ctx context.Context, modelAccess permission.UserAccess, userTag names.UserTag) error {
	// If the user tag is not local, we don't need to check for the model user
	// permissions. This is generally the case for macaroon-based logins.
	if !userTag.IsLocal() {
		return nil
	}

	// No model user found, so see if the user has been granted
	// access to the controller.
	if permission.IsEmptyUserAccess(modelAccess) {
		st := a.authContext.st
		controllerAccess, err := state.ControllerAccess(st, userTag)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.Trace(err)
		}
		// TODO(perrito666) remove the following section about everyone group
		// when groups are implemented, this accounts only for the lack of a local
		// ControllerUser when logging in from an external user that has not been granted
		// permissions on the controller but there are permissions for the special
		// everyone group.
		if permission.IsEmptyUserAccess(controllerAccess) && !userTag.IsLocal() {
			everyoneTag := names.NewUserTag(common.EveryoneTagName)
			controllerAccess, err = st.UserAccess(everyoneTag, st.ControllerTag())
			if err != nil && !errors.Is(err, errors.NotFound) {
				return errors.Annotatef(err, "obtaining ControllerUser for everyone group")
			}
		}
		if permission.IsEmptyUserAccess(controllerAccess) {
			return errors.NotFoundf("model or controller user")
		}
	}
	return nil
}

func (a *Authenticator) updateUserLastLogin(ctx context.Context, modelAccess permission.UserAccess, userTag names.UserTag, model *state.Model) error {
	updateLastLogin := func() error {
		// If the user is not local, we don't update the last login time.
		if !userTag.IsLocal() {
			return nil
		}

		// Update the last login time for the user.
		if err := a.authContext.userService.UpdateLastLogin(ctx, userTag.Name()); err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	if !permission.IsEmptyUserAccess(modelAccess) {
		if modelAccess.Object.Kind() != names.ModelTagKind {
			return errors.NotValidf("%s as model user", modelAccess.Object.Kind())
		}

		if err := model.UpdateLastModelConnection(modelAccess.UserTag); err != nil {
			// Attempt to update the users last login data, if the update
			// fails, then just report it as a log message and return the
			// original error message.
			if err := updateLastLogin(); err != nil {
				logger.Warningf("updating last login time for %v, %v", userTag, err)
			}
			return errors.Trace(err)
		}
	}

	if err := updateLastLogin(); err != nil {
		logger.Warningf("updating last login time for %v, %v", userTag, err)
	}

	return nil
}

func (a *Authenticator) isManager(entity state.Entity) bool {
	type withIsManager interface {
		IsManager() bool
	}

	m, ok := entity.(withIsManager)
	if !ok {
		return false
	}
	return m.IsManager()
}

// LoginRequest extracts basic auth login details from an http.Request.
//
// TODO(axw) we shouldn't be using params types here.
func LoginRequest(req *http.Request) (params.LoginRequest, error) {
	authHeader := req.Header.Get("Authorization")
	macaroons := httpbakery.RequestMacaroons(req)

	if authHeader == "" {
		return params.LoginRequest{Macaroons: macaroons}, nil
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
	if _, err := names.ParseTag(tagPass[0]); err != nil {
		return params.LoginRequest{}, errors.Trace(err)
	}

	bakeryVersion, _ := strconv.Atoi(req.Header.Get(httpbakery.BakeryProtocolHeader))
	loginRequest := params.LoginRequest{
		AuthTag:       tagPass[0],
		Credentials:   tagPass[1],
		Nonce:         req.Header.Get(params.MachineNonceHeader),
		Macaroons:     macaroons,
		BakeryVersion: bakery.Version(bakeryVersion),
	}
	// Default client version to 2 since older 2.x clients
	// don't send this field.
	requestClientVersion := version.Number{Major: 2}
	if clientVersion, err := common.JujuClientVersionFromRequest(req); err == nil {
		requestClientVersion = clientVersion
	}
	loginRequest.ClientVersion = requestClientVersion.String()
	return loginRequest, nil
}
