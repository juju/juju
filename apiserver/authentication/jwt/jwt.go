// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwt

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/permission"
)

// Authenticator defines an interface for authenticating requests.
type Authenticator interface {
	authentication.RequestAuthenticator
}

// TokenParser defines an interface for parsing JWT tokens.
type TokenParser interface {
	// Parse accepts a base64 string and extracts a JWT token.
	// This method should return a NotProvisioned error if
	// the parser is not ready.
	Parse(ctx context.Context, tok string) (jwt.Token, error)
}

// JWTAuthenticator is an authenticator responsible for handling JWT tokens from
// a client.
type JWTAuthenticator struct {
	parser TokenParser
}

// NewAuthenticator creates a new JWT authenticator with the supplied parser.
func NewAuthenticator(parser TokenParser) *JWTAuthenticator {
	return &JWTAuthenticator{
		parser: parser,
	}
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

// Authenticate implements EntityAuthenticator
func (j *JWTAuthenticator) Parse(ctx context.Context, tok string) (jwt.Token, TokenEntity, error) {
	token, err := j.parser.Parse(ctx, tok)
	if err != nil {
		// Return a not implemented error if the parser is not configured.
		// so that other authenticators are tried by the API server.
		if errors.Is(err, errors.NotProvisioned) {
			return nil, TokenEntity{}, errors.Trace(errors.NotImplemented)
		}
		return nil, TokenEntity{}, errors.Trace(err)
	}
	entity, err := userFromToken(token)
	if err != nil {
		return nil, TokenEntity{}, errors.Trace(err)
	}
	return token, entity, nil
}

// Authenticate implements EntityAuthenticator
func (j *JWTAuthenticator) Authenticate(req *http.Request) (authentication.AuthInfo, error) {
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		return authentication.AuthInfo{}, errors.NotFoundf("authorization header missing")
	}

	authScheme, rest, spaceFound := strings.Cut(authHeader, " ")
	if !spaceFound || authScheme != "Bearer" {
		// Invalid header format or no header provided.
		return authentication.AuthInfo{}, errors.NotFoundf("authorization header format")
	}
	rawToken, _, spaceFound := strings.Cut(rest, " ")
	if spaceFound {
		// Invalid header format or no header provided.
		return authentication.AuthInfo{}, errors.NotFoundf("authorization header format")
	}

	token, entity, err := j.Parse(req.Context(), rawToken)
	if err != nil {
		return authentication.AuthInfo{}, fmt.Errorf("parsing jwt: %w", err)
	}

	return authentication.AuthInfo{
		Entity:    entity,
		Delegator: &PermissionDelegator{token},
	}, nil
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
	case names.ApplicationOfferTagKind:
		validate = permission.ValidateOfferAccess
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
