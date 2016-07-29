// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package idmtest holds a mock implementation of the identity manager
// suitable for testing.
package idmtest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"

	"github.com/juju/httprequest"
	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery/agent"

	"github.com/juju/idmclient/params"
)

// Server represents a mock identity server.
// It currently serves only the discharge and groups endpoints.
type Server struct {
	// URL holds the URL of the mock identity server.
	// The discharger endpoint is located at URL/v1/discharge.
	URL *url.URL

	// PublicKey holds the public key of the mock identity server.
	PublicKey *bakery.PublicKey

	router *httprouter.Router
	srv    *httptest.Server
	bakery *bakery.Service

	// mu guards the fields below it.
	mu          sync.Mutex
	users       map[string]*user
	defaultUser string
	waits       []chan struct{}
}

type user struct {
	groups []string
	key    *bakery.KeyPair
}

// NewServer runs a mock identity server. It can discharge
// macaroons and return information on user group membership.
// The returned server should be closed after use.
func NewServer() *Server {
	srv := &Server{
		users: make(map[string]*user),
	}
	bsvc, err := bakery.NewService(bakery.NewServiceParams{
		Locator: srv,
	})
	if err != nil {
		panic(err)
	}
	srv.bakery = bsvc
	srv.PublicKey = bsvc.PublicKey()
	errorMapper := httprequest.ErrorMapper(errToResp)
	h := &handler{
		srv: srv,
	}
	router := httprouter.New()
	for _, route := range errorMapper.Handlers(func(httprequest.Params) (*handler, error) {
		return h, nil
	}) {
		router.Handle(route.Method, route.Path, route.Handle)
	}
	mux := http.NewServeMux()
	httpbakery.AddDischargeHandler(mux, "/", srv.bakery, srv.check)
	router.Handler("POST", "/v1/discharger/*rest", http.StripPrefix("/v1/discharger", mux))
	router.Handler("GET", "/v1/discharger/*rest", http.StripPrefix("/v1/discharger", mux))
	router.Handler("POST", "/discharge", mux)
	router.Handler("GET", "/publickey", mux)

	srv.srv = httptest.NewServer(router)
	srv.URL, err = url.Parse(srv.srv.URL)
	if err != nil {
		panic(err)
	}
	return srv
}

func errToResp(err error) (int, interface{}) {
	// Allow bakery errors to be returned as the bakery would
	// like them, so that httpbakery.Client.Do will work.
	if err, ok := errgo.Cause(err).(*httpbakery.Error); ok {
		return httpbakery.ErrorToResponse(err)
	}
	errorBody := errorResponseBody(err)
	status := http.StatusInternalServerError
	switch errorBody.Code {
	case params.ErrNotFound:
		status = http.StatusNotFound
	case params.ErrForbidden, params.ErrAlreadyExists:
		status = http.StatusForbidden
	case params.ErrBadRequest:
		status = http.StatusBadRequest
	case params.ErrUnauthorized, params.ErrNoAdminCredsProvided:
		status = http.StatusUnauthorized
	case params.ErrMethodNotAllowed:
		status = http.StatusMethodNotAllowed
	case params.ErrServiceUnavailable:
		status = http.StatusServiceUnavailable
	}
	return status, errorBody
}

// errorResponse returns an appropriate error response for the provided error.
func errorResponseBody(err error) *params.Error {
	errResp := &params.Error{
		Message: err.Error(),
	}
	cause := errgo.Cause(err)
	if coder, ok := cause.(errorCoder); ok {
		errResp.Code = coder.ErrorCode()
	} else if errgo.Cause(err) == httprequest.ErrUnmarshal {
		errResp.Code = params.ErrBadRequest
	}
	return errResp
}

type errorCoder interface {
	ErrorCode() params.ErrorCode
}

// Close shuts down the server.
func (srv *Server) Close() {
	srv.srv.Close()
}

// PublicKeyForLocation implements bakery.PublicKeyLocator
// by returning the server's public key for all locations.
func (srv *Server) PublicKeyForLocation(loc string) (*bakery.PublicKey, error) {
	return srv.PublicKey, nil
}

// UserPublicKey returns the key for the given user.
// It panics if the user has not been added.
func (srv *Server) UserPublicKey(username string) *bakery.KeyPair {
	u := srv.user(username)
	if u == nil {
		panic("no user found")
	}
	return u.key
}

// Client returns a bakery client that will discharge as the given user.
// If the user does not exist, it is added with no groups.
func (srv *Server) Client(username string) *httpbakery.Client {
	c := httpbakery.NewClient()
	u := srv.user(username)
	if u == nil {
		srv.AddUser(username)
		u = srv.user(username)
	}
	c.Key = u.key
	agent.SetUpAuth(c, srv.URL, username)
	return c
}

// SetDefaultUser configures the server so that it will discharge for
// the given user if no agent-login cookie is found. The user does not
// need to have been added. Note that this will bypass the
// VisitURL logic.
//
// If the name is empty, there will be no default user.
func (srv *Server) SetDefaultUser(name string) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.defaultUser = name
}

// AddUser adds a new user that's in the given set of groups.
// If the user already exists, the given groups are
// added to that user's groups.
func (srv *Server) AddUser(name string, groups ...string) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	u := srv.users[name]
	if u == nil {
		key, err := bakery.GenerateKey()
		if err != nil {
			panic(err)
		}
		srv.users[name] = &user{
			groups: groups,
			key:    key,
		}
		return
	}
	for _, g := range groups {
		found := false
		for _, ug := range u.groups {
			if ug == g {
				found = true
				break
			}
		}
		if !found {
			u.groups = append(u.groups, g)
		}
	}
}

func (srv *Server) user(name string) *user {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.users[name]
}

func (srv *Server) check(req *http.Request, cavId, cav string) ([]checkers.Caveat, error) {
	if cav != "is-authenticated-user" {
		return nil, errgo.Newf("unknown third party caveat %q", cav)
	}

	// First check if we have a login cookie so that we can avoid
	// going through the VisitURL logic when an explicit default user
	// has been set.
	username, key, err := agent.LoginCookie(req)
	if errgo.Cause(err) == agent.ErrNoAgentLoginCookie {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		if srv.defaultUser != "" {
			return []checkers.Caveat{
				checkers.DeclaredCaveat("username", srv.defaultUser),
			}, nil
		}
	}
	if err != nil {
		return nil, errgo.Notef(err, "bad agent-login cookie in request")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()

	waitId := len(srv.waits)
	srv.waits = append(srv.waits, make(chan struct{}, 1))
	// Return a visit URL so that the client code is forced through that
	// path, testing that its client correctly visits the URL and that
	// any agent-login cookie has been appropriately set.
	return nil, &httpbakery.Error{
		Code: httpbakery.ErrInteractionRequired,
		Info: &httpbakery.ErrorInfo{
			VisitURL: fmt.Sprintf("%s/v1/login/%d", srv.URL, waitId),
			WaitURL:  fmt.Sprintf("%s/v1/wait/%d?username=%s&caveat-id=%s&pubkey=%v", srv.URL, waitId, username, url.QueryEscape(cavId), url.QueryEscape(key.String())),
		},
	}
}

type handler struct {
	srv *Server
}

type groupsRequest struct {
	httprequest.Route `httprequest:"GET /v1/u/:User/groups"`
	User              string `httprequest:",path"`
}

func (h *handler) GetGroups(p httprequest.Params, req *groupsRequest) ([]string, error) {
	if err := h.checkRequest(p.Request); err != nil {
		return nil, err
	}
	if u := h.srv.user(req.User); u != nil {
		return u.groups, nil
	}
	return nil, params.ErrNotFound
}

func (h *handler) checkRequest(req *http.Request) error {
	_, err := httpbakery.CheckRequest(h.srv.bakery, req, nil, checkers.New())
	if err == nil {
		return nil
	}
	_, ok := errgo.Cause(err).(*bakery.VerificationError)
	if !ok {
		return err
	}
	m, err := h.srv.bakery.NewMacaroon("", nil, []checkers.Caveat{{
		Location:  h.srv.URL.String() + "/v1/discharger",
		Condition: "is-authenticated-user",
	}})
	if err != nil {
		panic(err)
	}
	return httpbakery.NewDischargeRequiredErrorForRequest(m, "", err, req)
}

type loginRequest struct {
	httprequest.Route `httprequest:"GET /v1/login/:WaitID"`
	WaitID            int `httprequest:",path"`
}

// TODO export VisitURLResponse from the bakery.
type visitURLResponse struct {
	AgentLogin bool `json:"agent_login"`
}

// serveLogin provides the /login endpoint. When /login is called it should
// be provided with a test id. /login also supports some additional parameters:
//     a = if set to "true" an agent URL will be added to the json response.
//     i = if set to "true" a plaintext response will be sent to simulate interaction.
func (h *handler) Login(p httprequest.Params, req *loginRequest) (*visitURLResponse, error) {
	h.srv.mu.Lock()
	defer h.srv.mu.Unlock()
	select {
	case h.srv.waits[req.WaitID] <- struct{}{}:
	default:
	}
	return &visitURLResponse{
		AgentLogin: true,
	}, nil
}

type waitRequest struct {
	httprequest.Route `httprequest:"GET /v1/wait/:WaitID"`
	WaitID            int               `httprequest:",path"`
	Username          string            `httprequest:"username,form"`
	CaveatID          string            `httprequest:"caveat-id,form"`
	PublicKey         *bakery.PublicKey `httprequest:"pubkey,form"`
}

func (h *handler) Wait(req *waitRequest) (*httpbakery.WaitResponse, error) {
	h.srv.mu.Lock()
	c := h.srv.waits[req.WaitID]
	h.srv.mu.Unlock()
	<-c
	u := h.srv.user(req.Username)
	if u == nil {
		return nil, errgo.Newf("user not found")
	}
	if *req.PublicKey != u.key.Public {
		return nil, errgo.Newf("public key mismatch")
	}
	checker := func(cavId, cav string) ([]checkers.Caveat, error) {
		return []checkers.Caveat{
			checkers.DeclaredCaveat("username", req.Username),
			bakery.LocalThirdPartyCaveat(&u.key.Public),
		}, nil
	}
	m, err := h.srv.bakery.Discharge(bakery.ThirdPartyCheckerFunc(checker), req.CaveatID)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot discharge", errgo.Any)
	}
	return &httpbakery.WaitResponse{
		Macaroon: m,
	}, nil
}
