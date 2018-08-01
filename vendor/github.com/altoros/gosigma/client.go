// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"crypto/tls"
	"errors"
	"time"

	"github.com/altoros/gosigma/https"
)

// A RequestSpec defines the type of client request
type RequestSpec bool

const (
	// RequestShort requests server to return short object representation
	RequestShort RequestSpec = false
	// RequestDetail requests server to return detail object representation
	RequestDetail RequestSpec = true
)

// A LibrarySpec defines the type of drive library to use
type LibrarySpec bool

const (
	// LibraryAccount requests server to use user account drive library
	LibraryAccount LibrarySpec = false
	// LibraryMedia requests server to use public drive library
	LibraryMedia LibrarySpec = true
)

// A Client sends and receives requests to CloudSigma endpoint
type Client struct {
	endpoint         string
	https            *https.Client
	logger           https.Logger
	operationTimeout time.Duration
}

var errEmptyUsername = errors.New("username is not allowed to be empty")
var errEmptyPassword = errors.New("password is not allowed to be empty")
var errEmptyUUID = errors.New("uuid is not allowed to be empty")

// NewClient returns new CloudSigma client object
func NewClient(endpoint string, username, password string,
	tlsConfig *tls.Config) (*Client, error) {

	endpoint = ResolveEndpoint(endpoint)

	if len(username) == 0 {
		return nil, errEmptyUsername
	}

	if len(password) == 0 {
		return nil, errEmptyPassword
	}

	client := &Client{
		endpoint: endpoint,
		https:    https.NewAuthClient(username, password, tlsConfig),
	}

	return client, nil
}

// ConnectTimeout sets connection timeout
func (c Client) ConnectTimeout(timeout time.Duration) {
	c.https.ConnectTimeout(timeout)
}

// GetConnectTimeout returns connection timeout for the object
func (c Client) GetConnectTimeout() time.Duration {
	return c.https.GetConnectTimeout()
}

// ReadWriteTimeout sets read-write timeout
func (c Client) ReadWriteTimeout(timeout time.Duration) {
	c.https.ReadWriteTimeout(timeout)
}

// GetReadWriteTimeout returns connection timeout for the object
func (c Client) GetReadWriteTimeout() time.Duration {
	return c.https.GetReadWriteTimeout()
}

// OperationTimeout sets timeout for cloud operations (like cloning, starting, stopping etc)
func (c *Client) OperationTimeout(timeout time.Duration) {
	c.operationTimeout = timeout
}

// GetOperationTimeout gets timeout for cloud operations (like cloning, starting, stopping etc)
func (c Client) GetOperationTimeout() time.Duration {
	return c.operationTimeout
}

// Logger sets logger for http traces
func (c *Client) Logger(logger https.Logger) {
	c.logger = logger
	c.https.Logger(logger)
}

// Servers in current account
func (c *Client) Servers(rqspec RequestSpec) ([]Server, error) {
	objs, err := c.getServers(rqspec)
	if err != nil {
		return nil, err
	}

	servers := make([]Server, len(objs))
	for i := 0; i < len(objs); i++ {
		servers[i] = &server{
			client: c,
			obj:    &objs[i],
		}
	}

	return servers, nil
}

// ServersFiltered in current account with filter applied
func (c *Client) ServersFiltered(rqspec RequestSpec, filter func(s Server) bool) ([]Server, error) {
	objs, err := c.getServers(rqspec)
	if err != nil {
		return nil, err
	}

	servers := make([]Server, 0, len(objs))
	for i := 0; i < len(objs); i++ {
		s := &server{
			client: c,
			obj:    &objs[i],
		}
		if filter(s) {
			servers = append(servers, s)
		}
	}

	return servers, nil
}

// Server returns given server by uuid, requesting endpoint for server information
func (c *Client) Server(uuid string) (Server, error) {
	obj, err := c.getServer(uuid)
	if err != nil {
		return nil, err
	}

	srv := &server{
		client: c,
		obj:    obj,
	}

	return srv, nil
}

// CreateServer in CloudSigma user account
func (c *Client) CreateServer(components Components) (Server, error) {
	objs, err := c.createServer(components)
	if err != nil {
		return nil, err
	}

	if len(objs) == 0 {
		return nil, errors.New("no servers in response from endpoint")
	}

	s := &server{
		client: c,
		obj:    &objs[0],
	}

	return s, nil
}

// StartServer by uuid of server instance.
func (c Client) StartServer(uuid string, avoid []string) error {
	return c.startServer(uuid, avoid)
}

// StopServer by uuid of server instance
func (c Client) StopServer(uuid string) error {
	return c.stopServer(uuid)
}

// RemoveServer by uuid of server instance with an option recursively removing attached drives.
// See RecurseXXX constants in server.go file.
func (c Client) RemoveServer(uuid, recurse string) error {
	return c.removeServer(uuid, recurse)
}

// Drives returns list of drives
func (c *Client) Drives(rqspec RequestSpec, libspec LibrarySpec) ([]Drive, error) {
	objs, err := c.getDrives(rqspec, libspec)
	if err != nil {
		return nil, err
	}

	drives := make([]Drive, len(objs))
	for i := 0; i < len(objs); i++ {
		drives[i] = &drive{
			client:  c,
			obj:     &objs[i],
			library: libspec,
		}
	}

	return drives, nil
}

// Drive returns given drive by uuid
func (c *Client) Drive(uuid string, libspec LibrarySpec) (Drive, error) {
	obj, err := c.getDrive(uuid, libspec)
	if err != nil {
		return nil, err
	}

	drv := &drive{
		client:  c,
		obj:     obj,
		library: libspec,
	}

	return drv, nil
}

// CloneDrive clones given drive by uuid
func (c *Client) CloneDrive(uuid string, libspec LibrarySpec, params CloneParams, avoid []string) (Drive, error) {
	obj, err := c.cloneDrive(uuid, libspec, params, avoid)
	if err != nil {
		return nil, err
	}

	drv := &drive{
		client:  c,
		obj:     obj,
		library: LibraryAccount,
	}

	return drv, nil
}

// RemoveDrive removes given drive by uuid
func (c *Client) RemoveDrive(uuid string, libspec LibrarySpec) error {
	return c.removeDrive(uuid, libspec)
}

// Job returns job object by uuid
func (c *Client) Job(uuid string) (Job, error) {
	obj, err := c.getJob(uuid)
	if err != nil {
		return nil, err
	}

	j := &job{
		client: c,
		obj:    obj,
	}

	return j, nil
}

// ReadContext reads and returns context of current server
func (c Client) ReadContext() (Context, error) {
	obj, err := c.readContext()
	if err != nil {
		return nil, err
	}

	ctx := context{obj: obj}

	return ctx, nil
}
