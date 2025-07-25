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
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/model"
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

// Authenticate implements EntityAuthenticator
func (j *JWTAuthenticator) Parse(ctx context.Context, tok string) (jwt.Token, names.Tag, error) {
	token, err := j.parser.Parse(ctx, tok)
	if err != nil {
		// Return a not implemented error if the parser is not configured.
		// so that other authenticators are tried by the API server.
		if errors.Is(err, errors.NotProvisioned) {
			return nil, nil, errors.Trace(errors.NotImplemented)
		}
		return nil, nil, errors.Trace(err)
	}
	userTag, err := userFromToken(token)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return token, userTag, nil
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

	token, userTag, err := j.Parse(req.Context(), rawToken)
	if err != nil {
		return authentication.AuthInfo{}, fmt.Errorf("parsing jwt: %w", err)
	}

	return authentication.AuthInfo{
		Tag:       userTag,
		Delegator: &PermissionDelegator{token},
	}, nil
}

// AuthenticateLoginRequest implements LoginAuthenticator
func (j *JWTAuthenticator) AuthenticateLoginRequest(
	ctx context.Context,
	_ string,
	_ model.UUID,
	authParams authentication.AuthParams,
) (authentication.AuthInfo, error) {
	if authParams.Token == "" {
		return authentication.AuthInfo{}, fmt.Errorf("auth token %w", errors.NotSupported)
	}

	token, userTag, err := j.Parse(ctx, authParams.Token)
	if err != nil {
		return authentication.AuthInfo{}, fmt.Errorf("parsing login access token: %w", err)
	}

	return authentication.AuthInfo{
		Tag:       userTag,
		Delegator: &PermissionDelegator{token},
	}, nil
}

// SubjectPermissions implements PermissionDelegator
func (p *PermissionDelegator) SubjectPermissions(
	_ context.Context,
	userName string,
	target permission.ID,
) (a permission.Access, err error) {
	if userName == permission.EveryoneUserName.Name() {
		// JWT auth process does not support everyone@external.
		// The everyone@external will be never included in the JWT token at least for now.
		return permission.NoAccess, nil
	}
	userTag, err := userFromToken(p.Token)
	if err != nil {
		return permission.NoAccess, errors.Trace(err)
	}
	// We need to make very sure that the entity the request pertains to
	// is the same entity this function was seeded with.
	if userTag.String() != names.NewUserTag(userName).String() {
		err = fmt.Errorf(
			"%w to use token permissions for one entity on another",
			apiservererrors.ErrPerm,
		)
		return permission.NoAccess, errors.WithType(err, authentication.ErrorEntityMissingPermission)
	}
	return PermissionFromToken(p.Token, target)
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

func userFromToken(token jwt.Token) (names.UserTag, error) {
	userTag, err := names.ParseUserTag(token.Subject())
	if err != nil {
		return names.UserTag{}, errors.Annotate(err, "invalid user tag in authToken")
	}
	return userTag, nil
}

// PermissionFromToken will extract the permission a jwt token has for the
// provided subject. If no permission is found permission.NoAccess will be
// returned.
func PermissionFromToken(token jwt.Token, subject permission.ID) (permission.Access, error) {
	var validate func(permission.Access) error
	switch subject.ObjectType {
	case permission.Controller:
		validate = permission.ValidateControllerAccess
	case permission.Model:
		validate = permission.ValidateModelAccess
	case permission.Cloud:
		validate = permission.ValidateCloudAccess
	case permission.Offer:
		validate = permission.ValidateOfferAccess
	default:
		return "", errors.NotValidf("%q as a target", subject)
	}
	accessClaims, ok := token.PrivateClaims()["access"].(map[string]interface{})
	if !ok || len(accessClaims) == 0 {
		return permission.NoAccess, nil
	}
	tag, err := permissionIDToTag(subject)
	if err != nil {
		return permission.NoAccess, err
	}
	access, ok := accessClaims[tag.String()]
	if !ok {
		return permission.NoAccess, nil
	}
	result := permission.Access(fmt.Sprintf("%v", access))
	return result, validate(result)
}

// permissionIDToTag returns a tag from a permission ID object.
func permissionIDToTag(id permission.ID) (names.Tag, error) {
	switch id.ObjectType {
	case permission.Cloud:
		if !names.IsValidCloud(id.Key) {
			return nil, fmt.Errorf("invalid cloud id %q", id.Key)
		}
		return names.NewCloudTag(id.Key), nil
	case permission.Controller:
		if !names.IsValidController(id.Key) {
			return nil, fmt.Errorf("invalid controller id %q", id.Key)
		}
		return names.NewControllerTag(id.Key), nil
	case permission.Model:
		if !names.IsValidModel(id.Key) {
			return nil, fmt.Errorf("invalid model id %q", id.Key)
		}
		return names.NewModelTag(id.Key), nil
	case permission.Offer:
		if !names.IsValidApplicationOffer(id.Key) {
			return nil, fmt.Errorf("invalid application offer id %q", id.Key)
		}
		return names.NewApplicationOfferTag(id.Key), nil
	default:
		return nil, errors.NotSupportedf("target id type %s", id.ObjectType)
	}
}
