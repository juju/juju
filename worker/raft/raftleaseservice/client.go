// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftleaseservice

import (
	"context"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/retry"
	"github.com/juju/utils/v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/raftleaseservice"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/pubsub/apiserver"
)

// MessageWriter defines the two methods called for sending lease messages.
type MessageWriter interface {
	// Send sends the given message to the server.
	Send(*params.LeaseOperation) error
	Close()
}

var dialOpts = api.DialOpts{
	DialAddressInterval: 20 * time.Millisecond,
	// If for some reason we are getting rate limited, there is a standard
	// five second delay before we get the login response. Ideally we need
	// to wait long enough for this response to get back to us.
	// Ideally the apiserver wouldn't be rate limiting connections from other
	// API servers, see bug #1733256.
	Timeout:    10 * time.Second,
	RetryDelay: 1 * time.Second,
}

// NewMessageWriter will connect to the remote defined by the info,
// and return a MessageWriter.
func NewMessageWriter(info *api.Info) (MessageWriter, error) {
	conn, err := api.Open(info, dialOpts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	a := raftleaseservice.NewAPI(conn)
	writer, err := a.OpenMessageWriter()
	if err != nil {
		conn.Close()
		return nil, errors.Trace(err)
	}
	return &remoteConnection{
		connection:    conn,
		MessageWriter: writer,
	}, nil
}

// remoteConnection represents an api connection to another
// API server for the purpose of forwarding pubsub messages.
type remoteConnection struct {
	connection api.Connection

	raftleaseservice.MessageWriter
}

func (r *remoteConnection) Close() {
	r.MessageWriter.Close()
	r.connection.Close()
}

// LeaseServiceClient defines a client for sending commands over to the raft
// lease service.
type LeaseServiceClient struct {
	mutex       sync.RWMutex
	info        *api.Info
	metrics     raftlease.ClientMetrics
	clock       clock.Clock
	unsubscribe func()
	logger      Logger

	newWriter      func(*api.Info) (MessageWriter, error)
	conn           MessageWriter
	stopConnecting chan struct{}
}

// LeaseServiceClientConfig holds resources and settings needed to run the
// LeaseServiceClient.
type LeaseServiceClientConfig struct {
	Hub           *pubsub.StructuredHub
	APIInfo       *api.Info
	NewWriter     func(*api.Info) (MessageWriter, error)
	ClientMetrics raftlease.ClientMetrics
	Clock         clock.Clock
	Logger        Logger
}

// NewLeaseServiceClient creates a new lease service client.
func NewLeaseServiceClient(config LeaseServiceClientConfig) (*LeaseServiceClient, error) {
	client := &LeaseServiceClient{
		info:      config.APIInfo,
		newWriter: config.NewWriter,
		metrics:   config.ClientMetrics,
		clock:     config.Clock,
		logger:    config.Logger,
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
	c.logger.Criticalf("Attempt Request %v", command)
	c.mutex.Lock()
	conn := c.conn
	c.mutex.Unlock()
	if conn == nil {
		if err := c.connect(); err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	bytes, err := command.Marshal()
	if err != nil {
		return errors.Trace(err)
	}

	uuid, err := utils.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	c.logger.Criticalf("Request %v", command)

	return c.conn.Send(&params.LeaseOperation{
		Command: string(bytes),
		UUID:    uuid.String(),
	})
}

func (c *LeaseServiceClient) Close() error {
	c.unsubscribe()
	c.interruptConnecting()
	if c.conn != nil {
		c.conn.Close()
	}
	return nil
}

func (c *LeaseServiceClient) apiserverDetailsChanged(topic string, details apiserver.Details, err error) {
	var addresses []string
	for _, server := range details.Servers {
		addresses = append(addresses, server.InternalAddress)
	}

	c.logger.Criticalf("ADDRESSES %v", addresses)

	// There are no addresses to try and change.
	if len(addresses) == 0 {
		return
	}

	c.mutex.Lock()
	c.info.Addrs = addresses
	c.mutex.Unlock()

	c.interruptConnecting()
	if err := c.connect(); err != nil {
		c.logger.Errorf("unable to get message writer %v", err)
	}
}

func (c *LeaseServiceClient) connect() error {
	stop := make(chan struct{})
	c.mutex.Lock()
	c.stopConnecting = stop

	info := c.info
	c.mutex.Unlock()

	c.logger.Debugf("connecting")

	var conn MessageWriter
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			c.logger.Debugf("open api to %v", info.Addrs)
			var err error
			conn, err = c.newWriter(info)
			if err != nil {
				c.logger.Tracef("unable to get message writer, reconnecting... : %v\n%s", err, errors.ErrorStack(err))
				return errors.Trace(err)
			}
			return nil
		},
		Attempts:    retry.UnlimitedAttempts,
		Delay:       time.Second,
		MaxDelay:    5 * time.Minute,
		BackoffFunc: retry.DoubleDelay,
		Stop:        stop,
		Clock:       c.clock,
	})
	if err != nil {
		return errors.Trace(err)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if conn != nil {
		c.conn = conn
		return nil
	}
	return errors.Errorf("unable to connect to %v", c.info.Addrs)
}

func (c *LeaseServiceClient) interruptConnecting() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.stopConnecting != nil {
		c.logger.Debugf("interrupting the pending connect loop")

		close(c.stopConnecting)
		c.stopConnecting = nil
	}
}
