// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwt

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/permission"
)

type Authenticator interface {
	authentication.RequestAuthenticator
	TokenParser
}

// JWTAuthenticator is an authenticator responsible for handling JWT tokens from
// a client.
type JWTAuthenticator struct {
	cache      *jwk.Cache
	httpClient *http.Client
	refreshURL string
}

// PermissionDelegator is responsible for handling authorization questions
// within the context of the JWT it has. It implements
// authentication.PermissionDelegator interface.
type PermissionDelegator struct {
	// Token is the authenticated context to answer all authorization questions
	// from.
	Token jwt.Token
}

// TokenEntity represents the entity found within a JWT token and conforms to
// state.Entity
type TokenEntity struct {
	User names.UserTag
}

// TokenParser parses a jwt token returning the token and
// entity derived from the token subject.
type TokenParser interface {
	// Parse parses the supplied token string and returns both the constructed
	// jwt and the entity found within the token.
	Parse(ctx context.Context, tok string) (jwt.Token, authentication.Entity, error)
}

func NewAuthenticator(refreshURL string) *JWTAuthenticator {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return NewAuthenticatorWithHTTPClient(httpClient, refreshURL)
}

func NewAuthenticatorWithHTTPClient(
	client *http.Client,
	refreshURL string,
) *JWTAuthenticator {
	return &JWTAuthenticator{
		httpClient: client,
		refreshURL: refreshURL,
	}
}

// Authenticate implements EntityAuthenticator
func (j *JWTAuthenticator) Authenticate(req *http.Request) (authentication.AuthInfo, error) {
	return authentication.AuthInfo{}, fmt.Errorf("jwt authenticate %w", errors.NotImplemented)
}

// AuthenticateLoginRequest implements LoginAuthenticator
func (j *JWTAuthenticator) AuthenticateLoginRequest(
	ctx context.Context,
	_, _ string,
	authParams authentication.AuthParams,
) (authentication.AuthInfo, error) {
	if authParams.Token == "" {
		return authentication.AuthInfo{}, fmt.Errorf("auth token %w", errors.NotSupported)
	}

	token, entity, err := j.Parse(ctx, authParams.Token)
	if err != nil {
		return authentication.AuthInfo{}, fmt.Errorf("parsing login access token: %w", err)
	}

	return authentication.AuthInfo{
		Entity:    entity,
		Delegator: &PermissionDelegator{token},
	}, nil
}

// Tag implements state.Entity
func (t TokenEntity) Tag() names.Tag {
	return t.User
}

// SubjectPermissions implements PermissionDelegator
func (p *PermissionDelegator) SubjectPermissions(
	e authentication.Entity,
	subject names.Tag,
) (a permission.Access, err error) {
	if e.Tag().Id() == common.EveryoneTagName {
		// JWT auth process does not support everyone@external.
		// The everyone@external will be never included in the JWT token at least for now.
		return permission.NoAccess, nil
	}
	tokenEntity, err := userFromToken(p.Token)
	if err != nil {
		return permission.NoAccess, errors.Trace(err)
	}
	// We need to make very sure that the entity the request pertains to
	// is the same entity this function was seeded with.
	if tokenEntity.Tag().String() != e.Tag().String() {
		err = fmt.Errorf(
			"%w to use token permissions for one entity on another",
			apiservererrors.ErrPerm,
		)
		return permission.NoAccess, errors.WithType(err, authentication.ErrorEntityMissingPermission)
	}
	return PermissionFromToken(p.Token, subject)
}

// PermissionsError implements PermissionDelegator
func (p *PermissionDelegator) PermissionError(
	subject names.Tag,
	perm permission.Access,
) error {
	return &apiservererrors.AccessRequiredError{
		RequiredAccess: map[names.Tag]permission.Access{
			subject: perm,
		},
	}
}

// Parse parses the bytes into a jwt.
func (j *JWTAuthenticator) Parse(ctx context.Context, tok string) (jwt.Token, authentication.Entity, error) {
	if j == nil || j.refreshURL == "" {
		return nil, nil, errors.New("no jwt authToken parser configured")
	}
	tokBytes, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return nil, nil, errors.Annotate(err, "invalid jwt authToken in request")
	}

	jwkSet, err := j.cache.Get(ctx, j.refreshURL)
	if err != nil {
		return nil, nil, errors.Annotate(err, "refreshing jwt key")
	}

	jwtTok, err := jwt.Parse(
		tokBytes,
		jwt.WithKeySet(jwkSet),
	)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	entity, err := userFromToken(jwtTok)
	return jwtTok, entity, err
}

// RegisterJWKSCache sets up the token key cache and refreshes the public key.
func (j *JWTAuthenticator) RegisterJWKSCache(ctx context.Context) error {
	j.cache = jwk.NewCache(ctx)

	err := j.cache.Register(j.refreshURL, jwk.WithHTTPClient(j.httpClient))
	if err != nil {
		return fmt.Errorf("registering jwk cache with url %q: %w", j.refreshURL, err)
	}
	_, err = j.cache.Refresh(ctx, j.refreshURL)
	if err != nil {
		return fmt.Errorf("refreshing jwk cache at %q: %w", j.refreshURL, err)
	}
	return nil
}

func userFromToken(token jwt.Token) (TokenEntity, error) {
	userTag, err := names.ParseUserTag(token.Subject())
	if err != nil {
		return TokenEntity{}, errors.Annotate(err, "invalid user tag in authToken")
	}
	return TokenEntity{userTag}, nil
}

// PermissionFromToken will extract the permission a jwt token has for the
// provided subject. If no permission is found permission.NoAccess will be
// returned.
func PermissionFromToken(token jwt.Token, subject names.Tag) (permission.Access, error) {
	var validate func(permission.Access) error
	switch subject.Kind() {
	case names.ControllerTagKind:
		validate = permission.ValidateControllerAccess
	case names.ModelTagKind:
		validate = permission.ValidateModelAccess
	case names.CloudTagKind:
		validate = permission.ValidateCloudAccess
	default:
		return "", errors.NotValidf("%q as a target", subject)
	}
	accessClaims, ok := token.PrivateClaims()["access"].(map[string]interface{})
	if !ok || len(accessClaims) == 0 {
		return permission.NoAccess, nil
	}
	access, ok := accessClaims[subject.String()]
	if !ok {
		return permission.NoAccess, nil
	}
	result := permission.Access(fmt.Sprintf("%v", access))
	return result, validate(result)
}
