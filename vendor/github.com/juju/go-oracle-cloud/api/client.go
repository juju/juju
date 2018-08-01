// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Config represents the significant details that a client
// needs in order to interact with the oracle cloud api.
type Config struct {
	// Identify will hold the oracle cloud client identify endpoint name
	Identify string

	// Username will hold the username oracle cloud client account
	Username string

	// Password will be the password of the orcale cloud client account
	Password string

	// Endpoint will hold the base url endpoint of the oracle cloud api
	Endpoint string
}

func (c Config) validate() error {
	if c.Identify == "" {
		return errors.New("go-oracle-cloud: Empty identify endpoint name")
	}

	if c.Username == "" {
		return errors.New("go-oracle-cloud: Empty client username")
	}

	if c.Password == "" {
		return errors.New("go-oracle-cloud: Empty client password")
	}

	if c.Endpoint == "" {
		return errors.New("go-oracle-cloud: Empty endpoint url basepath")
	}

	if _, err := url.ParseRequestURI(c.Endpoint); err != nil {
		return errors.New("go-oracle-cloud: The endpoint provided is invalid")
	}

	return nil
}

// Client holds the client credentials of the clients
// oracle cloud.
// The client needs identify name, user name and
// password in order to comunicate with the oracle
// cloud provider
type Client struct {
	// identify the intentity endpoint
	identify string
	// the username of the oracle account
	username string
	// the password of the oracle account
	password string
	// the endpoint of the oracle account
	endpoint string
	// internal http client
	http http.Client
	// intenrla map of endpoints of the entire
	// oracle cloud api
	endpoints map[string]string

	// mutex protects the cookie
	mutex *sync.Mutex
	// internal http cookie
	// this cookie will be generated based on the client connection
	cookie *http.Cookie
}

// NewClient returns a new client based on the cfg provided
func NewClient(cfg Config) (*Client, error) {
	var err error

	if err = cfg.validate(); err != nil {
		return nil, err
	}

	// if the endpoint contains the last char a "/" remove it
	index := strings.LastIndex(cfg.Endpoint, "/")
	if index == len(cfg.Endpoint)-1 {
		cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	}

	e := make(map[string]string, len(endpoints))
	for key := range endpoints {
		e[key] = fmt.Sprintf(endpoints[key], cfg.Endpoint)
	}

	cli := &Client{
		identify:  cfg.Identify,
		username:  cfg.Username,
		password:  cfg.Password,
		endpoint:  cfg.Endpoint,
		http:      http.Client{},
		endpoints: e,
		mutex:     &sync.Mutex{},
	}

	return cli, nil
}

// isAuth returns true if the cookie is set and present
// or false if not
func (c Client) isAuth() bool {
	if c.cookie == nil {
		return false
	}
	return true
}

// Idenitify return the identity name of the oracle cloud account
func (c Client) Identify() string {
	return c.identify
}

// Username returns the username of the oracle cloud account
func (c Client) Username() string {
	return c.username
}

// Password returns the password of the oracle cloud account
func (c Client) Password() string {
	return c.password
}

func (c Client) ComposeName(name string) string {
	return fmt.Sprintf("/Compute-%s/%s/%s",
		c.identify, c.username, name)
}

// RefreshCookie re authenticates the client into
// the oracle api
func (c *Client) RefreshCookie() (err error) {
	c.cookie = nil
	return c.Authenticate()
}

// RefreshToken refreshes the authentication token
// that expires usually around 30 minutes.
// This request extends the expiry of a valid authentication
// token by 30 minutes from the time you run the command.
// It extends the expiry of the current authentication token,
// but not beyond the session expiry time, which is 3 hours.
func (c *Client) RefreshToken() (err error) {
	url := c.endpoints["refreshtoken"] + "/"

	return c.request(paramsRequest{
		verb: "GET",
		url:  url,
		treat: func(resp *http.Response, verbRequest string) (err error) {
			if resp.StatusCode != http.StatusNoContent {
				return dumpApiError(resp)
			}

			// take the new refresh cookies
			cookies := resp.Cookies()
			if len(cookies) != 1 {
				return fmt.Errorf(
					"go-oracle-cloud: Invalid number of session cookies: %q",
					cookies,
				)
			}

			// take the cookie
			c.cookie = cookies[0]
			return nil
		},
	})
}
