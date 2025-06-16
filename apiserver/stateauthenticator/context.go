// Copyright 2015-2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/bakeryutil"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coremodel "github.com/juju/juju/core/model"
	corepermission "github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
	"github.com/juju/juju/state"
)

var errMacaroonAuthNotConfigured = errors.New("macaroon authentication is not configured")

const (
	// TODO make this configurable via model config.
	externalLoginExpiryTime = 24 * time.Hour
)

const (
	localUserIdentityLocationPath = "/auth"
)

// AccessService defines a interface for interacting the users and permissions
// of a controller.
type AccessService interface {
	// GetUserByAuth returns the user with the given name and password.
	GetUserByAuth(ctx context.Context, name coreuser.Name, password auth.Password) (coreuser.User, error)

	// GetUserByName returns the user with the given name.
	GetUserByName(ctx context.Context, name coreuser.Name) (coreuser.User, error)

	// UpdateLastModelLogin updates the last login time for the user with the
	// given name.
	UpdateLastModelLogin(ctx context.Context, name coreuser.Name, modelUUID coremodel.UUID) error

	// EnsureExternalUserIfAuthorized checks if an external user is missing from the
	// database and has permissions on an object. If they do then they will be
	// added. This ensures that juju has a record of external users that have
	// inherited their permissions from everyone@external.
	EnsureExternalUserIfAuthorized(ctx context.Context, subject coreuser.Name, target corepermission.ID) error

	// ReadUserAccessLevelForTarget returns the user access level for the given
	// user on the given target. A NotValid error is returned if the subject
	// (user) string is empty, or the target is not valid. Any errors from the
	// state layer are passed through. If the access level of a user cannot be
	// found then [accesserrors.AccessNotFound] is returned.
	ReadUserAccessLevelForTarget(ctx context.Context, subject coreuser.Name, target corepermission.ID) (corepermission.Access, error)
}

// AgentAuthenticatorGetter is a getter for creating authenticators, which
// can create authenticators for a given state.
type AgentAuthenticatorGetter interface {
	// Authenticator returns an authenticator using the controller model.
	Authenticator() authentication.EntityAuthenticator

	// AuthenticatorForModel returns an authenticator for the given model.
	AuthenticatorForModel(authentication.AgentPasswordService, *state.State) authentication.EntityAuthenticator
}

// authContext holds authentication context shared
// between all API endpoints.
type authContext struct {
	controllerConfigService ControllerConfigService
	accessService           AccessService
	macaroonService         MacaroonService

	clock           clock.Clock
	agentAuthGetter AgentAuthenticatorGetter

	// localUserBakery is the bakery.Bakery used by the controller
	// for authenticating local users. In time, we may want to use this for
	// both local and external users. Note that this service does not
	// discharge the third-party caveats.
	localUserBakery *bakeryutil.ExpirableStorageBakery

	// localUserThirdPartyBakery is the bakery.Bakery used by the
	// controller for discharging third-party caveats for local users.
	localUserThirdPartyBakery *bakery.Bakery
	// localUserThirdPartyBakeryKey is the bakery.Bakery's key.
	localUserThirdPartyBakeryKey *bakery.KeyPair

	// localUserInteractions maintains a set of in-progress local user
	// authentication interactions.
	localUserInteractions *authentication.Interactions

	// macaroonAuthOnce guards the fields below it.
	macaroonAuthOnce   sync.Once
	_macaroonAuth      *authentication.ExternalMacaroonAuthenticator
	_macaroonAuthError error
}

// OpenLoginAuthorizer authorises any login operation presented to it.
type OpenLoginAuthorizer struct{}

// AuthorizeOps implements OpsAuthorizer.AuthorizeOps.
func (OpenLoginAuthorizer) AuthorizeOps(ctx context.Context, authorizedOp bakery.Op, queryOps []bakery.Op) ([]bool, []checkers.Caveat, error) {
	logger.Debugf(ctx, "authorize query ops check for %v: %v", authorizedOp, queryOps)
	allowed := make([]bool, len(queryOps))
	for i := range allowed {
		allowed[i] = queryOps[i] == identchecker.LoginOp
	}
	return allowed, nil, nil
}

// newAuthContext creates a new authentication context for st.
func newAuthContext(
	ctx context.Context,
	controllerModelUUID coremodel.UUID,
	controllerConfigService ControllerConfigService,
	accessService AccessService,
	macaroonService MacaroonService,
	agentAuthGetter AgentAuthenticatorGetter,
	ctxClock clock.Clock,
) (*authContext, error) {
	ctxt := &authContext{
		clock:                   ctxClock,
		controllerConfigService: controllerConfigService,
		accessService:           accessService,
		macaroonService:         macaroonService,
		localUserInteractions:   authentication.NewInteractions(),
		agentAuthGetter:         agentAuthGetter,
	}

	// Create a bakery for discharging third-party caveats for
	// local user authentication. This service does not persist keys;
	// its macaroons should be very short-lived.
	checker := checkers.New(internalmacaroon.MacaroonNamespace)
	checker.Register("is-authenticated-user", internalmacaroon.MacaroonURI,
		// Having a macaroon with an is-authenticated-user
		// caveat is proof that the user is "logged in".
		// "is-authenticated-user",
		func(ctx context.Context, cond, arg string) error { return nil },
	)

	location := "juju model " + controllerModelUUID.String()
	var err error
	ctxt.localUserThirdPartyBakeryKey, err = macaroonService.GetLocalUsersThirdPartyKey(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "generating key for local user third party bakery key")
	}

	ctxt.localUserThirdPartyBakery = bakery.New(bakery.BakeryParams{
		Checker:       checker,
		Key:           ctxt.localUserThirdPartyBakeryKey,
		OpsAuthorizer: OpenLoginAuthorizer{},
		Location:      location,
	})

	// Create a bakery service for local user authentication. This service
	// persists keys into DQLite in a TTL collection.
	store := internalmacaroon.NewExpirableStorage(macaroonService, internalmacaroon.DefaultExpiration, ctxClock)
	locator := bakeryutil.BakeryThirdPartyLocator{
		PublicKey: ctxt.localUserThirdPartyBakeryKey.Public,
	}
	localUserBakeryKey, err := macaroonService.GetLocalUsersKey(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "generating key for local user bakery key")
	}
	localUserBakery := bakery.New(bakery.BakeryParams{
		RootKeyStore:  store,
		Key:           localUserBakeryKey,
		OpsAuthorizer: OpenLoginAuthorizer{},
		Location:      location,
	})

	ctxt.localUserBakery = &bakeryutil.ExpirableStorageBakery{
		Bakery:   localUserBakery,
		Location: location,
		Key:      localUserBakeryKey,
		Store:    store,
		Locator:  locator,
	}
	return ctxt, nil
}

// CreateLocalLoginMacaroon creates a macaroon that may be provided to a user
// as proof that they have logged in with a valid username and password. This
// macaroon may then be used to obtain a discharge macaroon so that the user
// can log in without presenting their password for a set amount of time.
func (ctxt *authContext) CreateLocalLoginMacaroon(ctx context.Context, tag names.UserTag, version bakery.Version) (*bakery.Macaroon, error) {
	return authentication.CreateLocalLoginMacaroon(ctx, tag, ctxt.localUserThirdPartyBakery.Oven, ctxt.clock, version)
}

// CheckLocalLoginCaveat parses and checks that the given caveat string is
// valid for a local login request, and returns the tag of the local user
// that the caveat asserts is logged in. checkers.ErrCaveatNotRecognized will
// be returned if the caveat is not recognised.
func (ctxt *authContext) CheckLocalLoginCaveat(caveat string) (names.UserTag, error) {
	return authentication.CheckLocalLoginCaveat(caveat)
}

type macaroonAuthFunc func(mss ...macaroon.Slice) *bakery.AuthChecker

func (f macaroonAuthFunc) Auth(_ context.Context, mss ...macaroon.Slice) *bakery.AuthChecker {
	return f(mss...)
}

// CheckLocalLoginRequest checks that the given HTTP request contains at least
// one valid local login macaroon minted using CreateLocalLoginMacaroon. It
// returns an error with a *bakery.VerificationError cause if the macaroon
// verification failed.
func (ctxt *authContext) CheckLocalLoginRequest(ctx context.Context, req *http.Request) error {
	return authentication.CheckLocalLoginRequest(ctx, macaroonAuthFunc(ctxt.localUserThirdPartyBakery.Checker.Auth), req)
}

// DischargeCaveats returns the caveats to add to a login discharge macaroon.
func (ctxt *authContext) DischargeCaveats(tag names.UserTag) []checkers.Caveat {
	return authentication.DischargeCaveats(tag, ctxt.clock)
}

// authenticatorForModel returns an authenticator.Authenticator for the API
// connection associated with the specified API server host and model.
func (ctxt *authContext) authenticatorForModel(serverHost string, agentPasswordService authentication.AgentPasswordService, st *state.State) authenticator {
	return authenticator{
		ctxt:               ctxt,
		serverHost:         serverHost,
		agentAuthenticator: ctxt.agentAuthGetter.AuthenticatorForModel(agentPasswordService, st),
	}
}

// authenticator returns an authenticator.Authenticator for the API
// connection associated with the specified API server host.
func (ctxt *authContext) authenticator(serverHost string) authenticator {
	return authenticator{
		ctxt:               ctxt,
		serverHost:         serverHost,
		agentAuthenticator: ctxt.agentAuthGetter.Authenticator(),
	}
}

// authenticator implements authenticator.Authenticator, delegating
// to the appropriate authenticator based on the tag kind.
type authenticator struct {
	ctxt               *authContext
	serverHost         string
	agentAuthenticator authentication.EntityAuthenticator
}

// Authenticate implements authentication.Authenticator
// by choosing the right kind of authentication for the given
// tag.
func (a authenticator) Authenticate(
	ctx context.Context,
	authParams authentication.AuthParams,
) (state.Entity, bool, error) {
	auth, err := a.authenticatorForTag(ctx, authParams.AuthTag)
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	return auth.Authenticate(ctx, authParams)
}

// authenticatorForTag returns the authenticator appropriate
// to use for a login with the given possibly-nil tag.
func (a authenticator) authenticatorForTag(ctx context.Context, tag names.Tag) (authentication.EntityAuthenticator, error) {
	if tag == nil || tag.Kind() == names.UserTagKind {
		// Poorly written older controllers pass in an external user
		// when doing api calls to the target controller during migration,
		// so we need to check the user type.
		if tag != nil && tag.(names.UserTag).IsLocal() {
			return a.localUserAuth(), nil
		}
		auth, err := a.ctxt.externalMacaroonAuth(ctx, nil)
		if errors.Cause(err) == errMacaroonAuthNotConfigured {
			err = errors.Trace(apiservererrors.ErrNoCreds)
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		return auth, nil
	}
	// If the tag is not a user tag, it must be an agent tag, attempt to locate
	// it.
	// TODO (stickupkid): This should just be a switch. We don't need to loop
	// through all the agent tags, it's pointless.
	for _, kind := range AgentTags {
		if tag.Kind() == kind {
			return a.agentAuthenticator, nil
		}
	}
	return nil, errors.Annotatef(apiservererrors.ErrBadRequest, "unexpected login entity tag")
}

// localUserAuth returns an authenticator that can authenticate logins for
// local users with either passwords or macaroons.
func (a authenticator) localUserAuth() *authentication.LocalUserAuthenticator {
	localUserIdentityLocation := url.URL{
		Scheme: "https",
		Host:   a.serverHost,
		Path:   localUserIdentityLocationPath,
	}
	return &authentication.LocalUserAuthenticator{
		UserService:               a.ctxt.accessService,
		Bakery:                    a.ctxt.localUserBakery,
		Clock:                     a.ctxt.clock,
		LocalUserIdentityLocation: localUserIdentityLocation.String(),
	}
}

// externalMacaroonAuth returns an authenticator that can authenticate macaroon-based
// logins for external users. If it fails once, it will always fail.
func (ctxt *authContext) externalMacaroonAuth(ctx context.Context, identClient identchecker.IdentityClient) (authentication.EntityAuthenticator, error) {
	ctxt.macaroonAuthOnce.Do(func() {
		ctxt._macaroonAuth, ctxt._macaroonAuthError = newExternalMacaroonAuth(ctx, externalMacaroonAuthenticatorConfig{
			controllerConfigService: ctxt.controllerConfigService,
			macaroonService:         ctxt.macaroonService,
			clock:                   ctxt.clock,
			expiryTime:              externalLoginExpiryTime,
			identClient:             identClient,
		})
	})
	if ctxt._macaroonAuth == nil {
		return nil, errors.Trace(ctxt._macaroonAuthError)
	}
	return ctxt._macaroonAuth, nil
}

type externalMacaroonAuthenticatorConfig struct {
	controllerConfigService ControllerConfigService
	macaroonService         MacaroonService
	clock                   clock.Clock
	expiryTime              time.Duration
	identClient             identchecker.IdentityClient
}

// newExternalMacaroonAuth returns an authenticator that can authenticate
// macaroon-based logins for external users. This is just a helper function
// for authCtxt.externalMacaroonAuth.
func newExternalMacaroonAuth(ctx context.Context, cfg externalMacaroonAuthenticatorConfig) (*authentication.ExternalMacaroonAuthenticator, error) {
	controllerCfg, err := cfg.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get controller config")
	}
	idURL := controllerCfg.IdentityURL()
	if idURL == "" {
		return nil, errMacaroonAuthNotConfigured
	}
	idPK := controllerCfg.IdentityPublicKey()
	key, err := cfg.macaroonService.GetExternalUsersThirdPartyKey(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	pkCache := bakery.NewThirdPartyStore()
	pkLocator := httpbakery.NewThirdPartyLocator(nil, pkCache)
	if idPK != nil {
		pkCache.AddInfo(idURL, bakery.ThirdPartyInfo{
			PublicKey: *idPK,
			Version:   3,
		})
	}

	auth := authentication.ExternalMacaroonAuthenticator{
		Clock:            cfg.clock,
		IdentityLocation: idURL,
	}
	store := internalmacaroon.NewExpirableStorage(cfg.macaroonService, cfg.expiryTime, cfg.clock)
	if cfg.identClient == nil {
		cfg.identClient = &auth
	}
	identBakery := identchecker.NewBakery(identchecker.BakeryParams{
		Checker:        httpbakery.NewChecker(),
		Locator:        pkLocator,
		Key:            key,
		IdentityClient: cfg.identClient,
		RootKeyStore:   store,
		Authorizer: identchecker.ACLAuthorizer{
			GetACL: func(ctx context.Context, op bakery.Op) ([]string, bool, error) {
				return []string{identchecker.Everyone}, false, nil
			},
		},
		Location: idURL,
	})
	auth.Bakery = identBakery
	return &auth, nil
}
