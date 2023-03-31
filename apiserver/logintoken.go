// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
)

type tokenEntity struct {
	user names.UserTag
}

func (t tokenEntity) Tag() names.Tag {
	return t.user
}

type jwtService struct {
	cache      *jwk.Cache
	refreshURL string
}

// RegisterJWKSCache sets up the token key cache and refreshes the public key.
func (j *jwtService) RegisterJWKSCache(ctx context.Context, client *http.Client) error {
	j.cache = jwk.NewCache(ctx)

	err := j.cache.Register(j.refreshURL, jwk.WithHTTPClient(client))
	if err != nil {
		return errors.Trace(err)
	}
	_, err = j.cache.Refresh(ctx, j.refreshURL)
	return errors.Trace(err)
}

// Parse parses the bytes into a jwt.
func (j *jwtService) Parse(ctx context.Context, tok string) (jwt.Token, state.Entity, error) {
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
		logger.Warningf("invalid jwt in request: %v", tok)
		return nil, nil, errors.Trace(err)
	}
	entity, err := userFromToken(jwtTok)
	return jwtTok, entity, err
}

func userFromToken(token jwt.Token) (state.Entity, error) {
	userTag, err := names.ParseUserTag(token.Subject())
	if err != nil {
		return nil, errors.Annotate(err, "invalid user tag in authToken")
	}
	return tokenEntity{userTag}, nil
}

func permissionFromToken(token jwt.Token, subject names.Tag) (permission.Access, error) {
	var validate func(permission.Access) error
	switch subject.Kind() {
	case names.ControllerTagKind:
		validate = permission.ValidateControllerAccess
	case names.ModelTagKind:
		validate = permission.ValidateModelAccess
	case names.ApplicationOfferTagKind:
		validate = permission.ValidateOfferAccess
	case names.CloudTagKind:
		validate = permission.ValidateCloudAccess
	default:
		return "", errors.NotValidf("%q as a target", subject)
	}
	accessClaims, ok := token.PrivateClaims()["access"].(map[string]interface{})
	if !ok || len(accessClaims) == 0 {
		logger.Warningf("authToken contains invalid access claims: %v", token.PrivateClaims()["access"])
		return permission.NoAccess, nil
	}
	access, ok := accessClaims[subject.String()]
	if !ok {
		return permission.NoAccess, nil
	}
	result := permission.Access(fmt.Sprintf("%v", access))
	return result, validate(result)
}
