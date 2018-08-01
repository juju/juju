// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package idmclient

import (
	"net/http"
	"net/url"
	"time"

	"github.com/juju/httprequest"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery/agent"

	"github.com/juju/idmclient/params"
)

// Note: tests for this code are in the server implementation.

const (
	Production = "https://api.jujucharms.com/identity"
	Staging    = "https://api.staging.jujucharms.com/identity"
)

// Client represents the client of an identity server.
type Client struct {
	client

	// permChecker is used to check group membership.
	// It is only non-zero when groups are enabled.
	permChecker *PermChecker
}

var _ IdentityClient = (*Client)(nil)

// NewParams holds the parameters for creating a new client.
type NewParams struct {
	// BaseURL holds the URL of the identity manager.
	BaseURL string

	// Client holds the client to use to make requests
	// to the identity manager.
	Client *httpbakery.Client

	// AgentUsername holds the username for group-fetching authorization.
	// If this is empty, no group information will be provided.
	// The agent key is expected to be held inside the Client.
	AgentUsername string

	// CacheTime holds the maximum duration for which
	// group membership information will be cached.
	// If this is zero, group membership information will not be cached.
	CacheTime time.Duration
}

// New returns a new client.
func New(p NewParams) (*Client, error) {
	var c Client
	u, err := url.Parse(p.BaseURL)
	if p.BaseURL == "" || err != nil {
		return nil, errgo.Newf("bad identity client base URL %q", p.BaseURL)
	}
	c.Client.BaseURL = p.BaseURL
	if p.AgentUsername != "" {
		if err := agent.SetUpAuth(p.Client, u, p.AgentUsername); err != nil {
			return nil, errgo.Notef(err, "cannot set up agent authentication")
		}
		c.permChecker = NewPermChecker(&c, p.CacheTime)
	}
	c.Client.Doer = p.Client
	c.Client.UnmarshalError = httprequest.ErrorUnmarshaler(new(params.Error))
	return &c, nil
}

// IdentityCaveat implements IdentityClient.IdentityCaveat.
func (c *Client) IdentityCaveats() []checkers.Caveat {
	return []checkers.Caveat{
		checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Location:  c.Client.BaseURL,
				Condition: "is-authenticated-user",
			},
			"username",
		),
	}
}

// DeclaredIdentity implements IdentityClient.DeclaredIdentity.
// On success, it returns a value of type *User.
func (c *Client) DeclaredIdentity(declared map[string]string) (Identity, error) {
	username := declared["username"]
	if username == "" {
		return nil, errgo.Newf("no declared user name in %q", declared)
	}
	return &User{
		client:   c,
		username: username,
	}, nil
}

// UserDeclaration returns a first party caveat that can be used
// by an identity manager to declare an identity on a discharge
// macaroon.
func UserDeclaration(username string) checkers.Caveat {
	return checkers.DeclaredCaveat("username", username)
}

// CacheEvict evicts username from the user info cache.
func (c *Client) CacheEvict(username string) {
	if c.permChecker != nil {
		c.permChecker.CacheEvict(username)
	}
}

// CacheEvictAll evicts everything from the user info cache.
func (c *Client) CacheEvictAll() {
	if c.permChecker != nil {
		c.permChecker.CacheEvictAll()
	}
}

// User is an implementation of Identity that also implements
// IDM-specific methods for obtaining the user name and
// group membership information.
type User struct {
	client   *Client
	username string
}

// Username returns the user name of the user.
func (u *User) Username() (string, error) {
	return u.username, nil
}

// Groups returns all the groups that the user
// is a member of.
//
// Note: use of this method should be avoided if
// possible, as a user may potentially be in huge
// numbers of groups.
func (u *User) Groups() ([]string, error) {
	if u.client.permChecker != nil {
		return u.client.permChecker.cache.Groups(u.username)
	}
	return nil, nil
}

// Allow reports whether the user should be allowed to access
// any of the users or groups in the given ACL slice.
func (u *User) Allow(acl []string) (bool, error) {
	if u.client.permChecker != nil {
		return u.client.permChecker.Allow(u.username, acl)
	}
	// No groups - just implement the trivial cases.
	for _, name := range acl {
		if name == "everyone" || name == u.username {
			return true, nil
		}
	}
	return false, nil
}

// Id implements Identity.Id. Currently it just returns the user
// name, but user code should not rely on it doing that - eventually
// it will return an opaque user identifier rather than the user name.
func (u *User) Id() string {
	return u.username
}

// Domain implements Identity.Domain.
// Currently it always returns the empty domain.
func (u *User) Domain() string {
	return ""
}

// PublicKey implements Identity.PublicKey.
// Currently it always returns nil as identity
// declaration caveats are not encrypted.
func (u *User) PublicKey() *bakery.PublicKey {
	return nil
}

// LoginMethods returns information about the available login methods
// for the given URL, which is expected to be a URL as passed to
// a VisitWebPage function during the macaroon bakery discharge process.
func LoginMethods(client *http.Client, u *url.URL) (*params.LoginMethods, error) {
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, errgo.Notef(err, "cannot create request")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, errgo.Notef(err, "cannot do request")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var herr httpbakery.Error
		if err := httprequest.UnmarshalJSONResponse(resp, &herr); err != nil {
			return nil, errgo.Notef(err, "cannot unmarshal error")
		}
		return nil, &herr
	}
	var lm params.LoginMethods
	if err := httprequest.UnmarshalJSONResponse(resp, &lm); err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal login methods")
	}
	return &lm, nil
}

//go:generate httprequest-generate-client $IDM_SERVER_REPO/internal/v1 apiHandler client
