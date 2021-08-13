// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftleaseservice

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/retry"
	"github.com/juju/utils"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/pubsub/apiserver"
)

// LeaseServiceClient defines a client for sending commands over to the raft
// lease service.
type LeaseServiceClient struct {
	mutex          sync.RWMutex
	apiInfo        *api.Info
	addresses      []string
	tlsConfig      *tls.Config
	path           string
	metrics        raftlease.ClientMetrics
	clock          clock.Clock
	unsubscribe    func()
	conn           *websocket.Conn
	requestTimeout time.Duration
}

// LeaseServiceClientConfig holds resources and settings needed to run the
// LeaseServiceClient.
type LeaseServiceClientConfig struct {
	Hub            *pubsub.StructuredHub
	APIInfo        *api.Info
	TLSConfig      *tls.Config
	Path           string
	RequestTimeout time.Duration
	ClientMetrics  raftlease.ClientMetrics
	Clock          clock.Clock
}

// NewLeaseServiceClient creates a new lease service client.
func NewLeaseServiceClient(config LeaseServiceClientConfig) (*LeaseServiceClient, error) {
	client := &LeaseServiceClient{
		apiInfo: config.APIInfo,
		// Ensure we have at least some addresses, as pubsub may fail us and we
		// need to have some addresses before making a request.
		addresses:      config.APIInfo.Addrs,
		tlsConfig:      config.TLSConfig,
		path:           config.Path,
		requestTimeout: config.RequestTimeout,
		metrics:        config.ClientMetrics,
		clock:          config.Clock,
	}

	// Watch for apiserver details changes.
	var err error
	client.unsubscribe, err = config.Hub.Subscribe(apiserver.DetailsTopic, client.apiserverDetailsChanged)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Ask for the current details to be sent.
	req := apiserver.DetailsRequest{
		Requester: "lease-service-client",
	}
	if _, err := config.Hub.Publish(apiserver.DetailsRequestTopic, req); err != nil {
		return nil, errors.Trace(err)
	}

	return client, nil
}

func (c *LeaseServiceClient) Request(ctx context.Context, command *raftlease.Command) error {
	bytes, err := command.Marshal()
	if err != nil {
		return errors.Trace(err)
	}

	// TODO (stickupkid): We need to serialize the requests, so that only
	// one request can happen at one time.

	// We have an existing connection attempt to use that. If that isn't
	// successful, fallback to iterating each address.
	if c.conn != nil {
		if err := c.do(ctx, c.conn, bytes); err == nil {
			return nil
		}
	}

	// Unfortunately we don't have a cached connection, instead we now have to
	// iterate the list of address to find one that we can send our command to.
	urls := c.constructURLs()

	return retry.Call(retry.CallArgs{
		Func: func() error {
			url := urls[len(urls)]
			conn, err := c.dial(ctx, url)
			if err != nil {
				return errors.Trace(err)
			}

			if err = c.do(ctx, conn, bytes); err == nil {
				// Cache the request so that we don't have to perform this
				// request everytime.
				c.conn = conn
			}
			return errors.Trace(err)
		},
		IsFatalError: func(err error) bool {
			return errors.IsBadRequest(err)
		},
		MaxDuration: c.requestTimeout,
	})
}

func (c *LeaseServiceClient) Close() error {
	c.unsubscribe()
	if c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

func (c *LeaseServiceClient) do(ctx context.Context, conn *websocket.Conn, command []byte) error {
	applyTicker := c.clock.After(c.requestTimeout)

	// Generate a new unique UUID for every request.
	uuid, err := utils.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	writeOp := params.LeaseOperation{
		Command: string(command),
		UUID:    uuid.String(),
	}
	if err := conn.WriteJSON(&writeOp); err != nil {
		return errors.Trace(err)
	}

	var readOp params.LeaseOperationResult
	for {
		select {
		case <-applyTicker:
			return errors.Errorf("timeout requesting")
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := conn.ReadJSON(&readOp); err != nil {
			return errors.Trace(err)
		}
		if readOp.UUID == uuid.String() {
			break
		}
	}
	return readOp.Error
}

// constructURLs turns a series of addresses into websocket urls that can
// query the service endpoint.
func (c *LeaseServiceClient) constructURLs() []*url.URL {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	urls := make([]*url.URL, len(c.addresses))
	for k, address := range c.addresses {
		urls[k] = &url.URL{
			Scheme: "wss",
			Host:   address,
			Path:   c.path,
		}
	}

	return urls
}

func (c *LeaseServiceClient) dial(ctx context.Context, url *url.URL) (*websocket.Conn, error) {
	header, err := c.dialHeaders()
	if err != nil {
		return nil, errors.Trace(err)
	}

	dialer := websocket.Dialer{
		TLSClientConfig: c.tlsConfig,
	}
	conn, resp, err := dialer.DialContext(ctx, url.String(), header)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return conn, nil
	case http.StatusBadRequest:
		// The content mad this terminal.
		_ = conn.Close()
		return nil, errors.BadRequestf("request was invalid")
	}
	_ = conn.Close()
	return nil, errors.NotFoundf("%v", url)
}

func (c *LeaseServiceClient) apiserverDetailsChanged(topic string, details apiserver.Details, err error) {
	var addresses []string
	for _, server := range details.Servers {
		addresses = append(addresses, server.InternalAddress)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.addresses = addresses
}

func (c *LeaseServiceClient) dialHeaders() (http.Header, error) {
	header := http.Header{}
	if tag := c.apiInfo.Tag.String(); tag != "" {
		// Note that password may be empty here; we still
		// want to pass the tag along. An empty password
		// indicates that we're using macaroon authentication.
		api.SetBasicAuthHeader(header, tag, c.apiInfo.Password)
	}
	if err := api.AuthHTTPHeader(header, c.apiInfo.Nonce, c.apiInfo.Macaroons); err != nil {
		return nil, errors.Trace(err)
	}
	return header, nil
}
