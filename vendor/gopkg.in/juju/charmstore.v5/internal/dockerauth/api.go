// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dockerauth

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/juju/loggo"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	errgo "gopkg.in/errgo.v1"
	httprequest "gopkg.in/httprequest.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	"gopkg.in/macaroon.v2-unstable"

	"gopkg.in/juju/charmstore.v5/internal/charmstore"
)

var logger = loggo.GetLogger("charmstore.internal.dockerauth")

// parseResourceAccess parses the requested access for a single resource
// from a scope. This is parsed as a resourcescope from the grammer
// specified in
// https://docs.docker.com/registry/spec/auth/scope/#resource-scope-grammar.
func parseResourceAccessRights(s string) (resourceAccessRights, error) {
	var ra resourceAccessRights
	i := strings.IndexByte(s, ':')
	j := strings.LastIndexByte(s, ':')
	if i == j {
		return ra, errgo.Newf("invalid resource scope %q", s)
	}
	ra.Type = s[:i]
	ra.Name = s[i+1 : j]
	actions := s[j+1:]
	if actions != "" {
		ra.Actions = strings.Split(actions, ",")
	}
	return ra, nil
}

// parseScope parses a requested scope and returns the set or requested
// resource accesses. An error of type *ScopeParseError is returned if
// any part of the scope is not valid. Any valid resource scopes are
// always returned.
func parseScope(s string) ([]resourceAccessRights, error) {
	if s == "" {
		return nil, nil
	}
	var ras []resourceAccessRights
	for _, rights := range strings.Split(s, " ") {
		ra, err := parseResourceAccessRights(rights)
		if err != nil {
			return nil, errgo.Notef(err, "invalid access rights in resource scope %q", s)
		}
		ras = append(ras, ra)
	}
	return ras, nil
}

type Handler struct {
	params charmstore.APIHandlerParams
}

type tokenRequest struct {
	httprequest.Route `httprequest:"GET /token"`
	Scope             string `httprequest:"scope,form"`
	Service           string `httprequest:"service,form"`
}

type tokenResponse struct {
	Token     string    `json:"token"`
	ExpiresIn int       `json:"expires_in"`
	IssuedAt  time.Time `json:"issued_at"`
}

type dockerRegistryClaims struct {
	jwt.StandardClaims
	Access []resourceAccessRights `json:"access"`
}

func (h *Handler) handler(p httprequest.Params) (*handler, context.Context, error) {
	store, err := h.params.Pool.RequestStore()
	if err != nil {
		return nil, p.Context, errgo.Mask(err)
	}
	return &handler{h, store}, p.Context, nil
}

type handler struct {
	h     *Handler
	store *charmstore.Store
}

func (h *handler) Close() error {
	h.store.Close()
	return nil
}

// Token implements the token issuing endpoint for a docker registry
// authorization service. See
// https://docs.docker.com/registry/spec/auth/token/ for the protocol
// details.
func (h *handler) Token(p httprequest.Params, req *tokenRequest) (*tokenResponse, error) {
	ras, err := parseScope(req.Scope)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	ms := credentials(p.Request)
	filteredRAs := make([]resourceAccessRights, 0, len(ras))
	for _, ra := range ras {
		if ra.Type != "repository" {
			continue
		}
		repoChecker := dockerRepoChecker{
			repoName: ra.Name,
		}
		filteredActions := make([]string, 0, len(ra.Actions))
		for _, a := range ra.Actions {
			// Note: possible values for a include "push" and "pull".
			err := h.store.Bakery.Check(ms, checkers.New(checkers.OperationChecker(a), repoChecker, checkers.TimeBefore))
			if err == nil {
				filteredActions = append(filteredActions, a)
			} else {
				logger.Debugf("docker token check failed for operation %q: %v", a, err)
			}
		}
		if len(filteredActions) == 0 {
			continue
		}
		filteredRAs = append(filteredRAs, resourceAccessRights{
			Type:    ra.Type,
			Name:    ra.Name,
			Actions: filteredActions,
		})
	}
	issuedAt := time.Now()
	s, err := h.createToken(filteredRAs, req.Service, issuedAt)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &tokenResponse{
		Token:     s,
		ExpiresIn: int(h.h.params.DockerRegistryTokenDuration / time.Second),
		IssuedAt:  issuedAt,
	}, nil
}

type dockerRepoChecker struct {
	repoName string
}

func (c dockerRepoChecker) Condition() string {
	return "is-docker-repo"
}

func (c dockerRepoChecker) Check(_, arg string) error {
	if c.repoName != arg {
		return errgo.Newf("invalid repository name; got %q want %q", c.repoName, arg)
	}
	return nil
}

func credentials(req *http.Request) macaroon.Slice {
	_, pw, _ := req.BasicAuth()
	b, err := base64.RawStdEncoding.DecodeString(pw)
	if err != nil {
		logger.Debugf("invalid macaroon: %s", err)
		return nil
	}
	var ms macaroon.Slice
	if err := ms.UnmarshalBinary(b); err != nil {
		logger.Debugf("invalid macaroon: %s", err)
		return nil
	}
	return ms
}

// createToken creates a JWT for the given service with the givne access
// rights.
func (h *handler) createToken(ras []resourceAccessRights, service string, issuedAt time.Time) (string, error) {
	var issuer string
	if len(h.h.params.DockerRegistryAuthCertificates) > 0 {
		issuer = h.h.params.DockerRegistryAuthCertificates[0].Subject.CommonName
	}
	expiresAt := issuedAt.Add(h.h.params.DockerRegistryTokenDuration)
	claims := dockerRegistryClaims{
		StandardClaims: jwt.StandardClaims{
			Audience:  service,
			ExpiresAt: expiresAt.Unix(),
			IssuedAt:  issuedAt.Unix(),
			NotBefore: issuedAt.Unix(),
			Issuer:    issuer,
		},
		Access: ras,
	}
	var sm jwt.SigningMethod
	switch h.h.params.DockerRegistryAuthKey.(type) {
	case *ecdsa.PrivateKey:
		sm = jwt.SigningMethodES256
	case *rsa.PrivateKey:
		sm = jwt.SigningMethodRS256
	default:
		sm = jwt.SigningMethodNone
	}
	tok := jwt.NewWithClaims(sm, claims)
	certs := make([]string, len(h.h.params.DockerRegistryAuthCertificates))
	for i, c := range h.h.params.DockerRegistryAuthCertificates {
		certs[i] = base64.StdEncoding.EncodeToString(c.Raw)
	}
	// The x5c header contains the certificate chain used by the
	// docker-registry to authenticate the token.
	tok.Header["x5c"] = certs
	s, err := tok.SignedString(h.h.params.DockerRegistryAuthKey)
	if err != nil {
		return "", errgo.Mask(err)
	}
	return s, nil
}

func NewAPIHandler(p charmstore.APIHandlerParams) (charmstore.HTTPCloseHandler, error) {
	logger.Infof("Adding docker-registry")
	h := &Handler{
		params: p,
	}
	r := httprouter.New()
	srv := httprequest.Server{
		ErrorMapper: errorMapper,
	}
	httprequest.AddHandlers(r, srv.Handlers(h.handler))
	return server{
		Handler: r,
	}, nil
}

type server struct {
	http.Handler
}

func (server) Close() {}

func errorMapper(ctx context.Context, err error) (httpStatus int, errorBody interface{}) {
	// TODO return docker-registry standard error format (see
	// https://docs.docker.com/registry/spec/api/#errors)
	return http.StatusInternalServerError, &httprequest.RemoteError{
		Message: err.Error(),
	}
}
