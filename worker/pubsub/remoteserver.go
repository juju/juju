// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/retry"
	"github.com/juju/utils/deque"
	"github.com/juju/worker/v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/pubsub/forwarder"
)

// RemoteServer represents the public interface of the worker
// responsible for forwarding messages to a single other API server.
type RemoteServer interface {
	worker.Worker
	Reporter
	UpdateAddresses(addresses []string)
	Publish(message *params.PubSubMessage)
}

// remoteServer is responsible for taking messages and sending them to the
// pubsub endpoint on the remote server. If the connection is dropped, the
// remoteServer will try to reconnect. Messages are not sent until the
// connection either succeeds the first time, or fails to connect. Once there
// is a failure, incoming messages are dropped until reconnection is complete,
// then messages will flow again.
type remoteServer struct {
	origin string
	target string
	info   *api.Info
	logger Logger

	newWriter  func(*api.Info) (MessageWriter, error)
	connection MessageWriter

	hub   *pubsub.StructuredHub
	tomb  tomb.Tomb
	clock clock.Clock
	mutex sync.Mutex

	pending         *deque.Deque
	data            chan struct{}
	stopConnecting  chan struct{}
	connectionReset chan struct{}
	sent            uint64

	unsubscribe func()
}

// RemoteServerConfig defines all the attributes that are needed for a RemoteServer.
type RemoteServerConfig struct {
	// Hub is used to publish connection messages
	Hub    *pubsub.StructuredHub
	Origin string
	Target string
	Clock  clock.Clock
	Logger Logger

	// APIInfo is initially populated with the addresses of the target machine.
	APIInfo   *api.Info
	NewWriter func(*api.Info) (MessageWriter, error)
}

// NewRemoteServer creates a new RemoteServer that will connect to the remote
// apiserver and pass on messages to the pubsub endpoint of that apiserver.
func NewRemoteServer(config RemoteServerConfig) (RemoteServer, error) {
	remote := &remoteServer{
		origin:    config.Origin,
		target:    config.Target,
		info:      config.APIInfo,
		logger:    config.Logger,
		newWriter: config.NewWriter,
		hub:       config.Hub,
		clock:     config.Clock,
		pending:   deque.New(),
		data:      make(chan struct{}),
	}
	unsub, err := remote.hub.Subscribe(forwarder.ConnectedTopic, remote.onForwarderConnection)
	if err != nil {
		return nil, errors.Trace(err)
	}
	remote.unsubscribe = unsub
	remote.tomb.Go(remote.loop)
	return remote, nil
}

// Report provides information to the engine report.
// It should be fast and minimally blocking.
func (r *remoteServer) Report() map[string]interface{} {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var status string
	if r.connection == nil {
		status = "disconnected"
	} else {
		status = "connected"
	}
	return map[string]interface{}{
		"status":    status,
		"addresses": r.info.Addrs,
		"queue-len": r.pending.Len(),
		"sent":      r.sent,
	}
}

// IntrospectionReport is the method called by the subscriber to get
// information about this server.
func (r *remoteServer) IntrospectionReport() string {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var status string
	if r.connection == nil {
		status = "disconnected"
	} else {
		status = "connected"
	}
	return fmt.Sprintf(""+
		"  Status: %s\n"+
		"  Addresses: %v\n"+
		"  Queue length: %d\n"+
		"  Sent count: %d\n",
		status, r.info.Addrs, r.pending.Len(), r.sent)
}

func (r *remoteServer) onForwarderConnection(topic string, details forwarder.OriginTarget, err error) {
	if err != nil {
		// This should never happen.
		r.logger.Errorf("subscriber callback error: %v", err)
		return
	}
	if details.Target == r.origin && details.Origin == r.target {
		// If we have just been connected to by the apiserver that we are
		// trying to connect to, interrupt any waiting we may be doing and try
		// again as we may be in the middle of a long wait.
		r.interruptConnecting()
	}
}

// UpdateAddresses will update the addresses held for the target API server.
// If we are currently trying to connect to the target, interrupt it so we
// can try again with the new addresses.
func (r *remoteServer) UpdateAddresses(addresses []string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.connection == nil && r.stopConnecting != nil {
		// We are probably trying to reconnect, so interrupt that so we don't
		// get a race between setting addresses and trying to read them to
		// connect. Note that we don't call the interruptConnecting method
		// here because that method also tries to lock the mutex.
		r.logger.Debugf("interrupting connecting due to new addresses: %v", addresses)
		close(r.stopConnecting)
		r.stopConnecting = nil
	}
	r.info.Addrs = addresses
}

// Publish queues up the message if and only if we have an active connection to
// the target apiserver.
func (r *remoteServer) Publish(message *params.PubSubMessage) {
	select {
	case <-r.tomb.Dying():
		r.logger.Tracef("dying, don't send %q", message.Topic)
	default:
		r.mutex.Lock()
		// Only queue the message up if we are currently connected.
		notifyData := false
		if r.connection != nil {
			r.logger.Tracef("queue up topic %q", message.Topic)
			r.pending.PushBack(message)
			notifyData = r.pending.Len() == 1

		} else {
			r.logger.Tracef("skipping %q for %s as not connected", message.Topic, r.target)
		}
		r.mutex.Unlock()
		if notifyData {
			select {
			case r.data <- struct{}{}:
			case <-r.connectionReset:
				r.logger.Debugf("connection reset while notifying %q for %s", message.Topic, r.target)
			}
		}
	}
}

// nextMessage returns the next queued message, and a flag to indicate empty.
func (r *remoteServer) nextMessage() *params.PubSubMessage {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	val, ok := r.pending.PopFront()
	if !ok {
		// nothing to do
		return nil
	}
	// Even though it isn't exactly sent right now, it effectively will
	// be very soon, and we want to keep this counter in the mutex lock.
	r.sent++
	return val.(*params.PubSubMessage)
}

func (r *remoteServer) connect() bool {
	stop := make(chan struct{})
	r.mutex.Lock()
	r.stopConnecting = stop
	r.mutex.Unlock()

	var connection MessageWriter
	r.logger.Debugf("connecting to %s", r.target)
	retry.Call(retry.CallArgs{
		Func: func() error {
			r.logger.Debugf("open api to %s: %v", r.target, r.info.Addrs)
			conn, err := r.newWriter(r.info)
			if err != nil {
				r.logger.Tracef("unable to get message writer for %s, reconnecting... : %v\n%s", r.target, err, errors.ErrorStack(err))
				return errors.Trace(err)
			}
			connection = conn
			return nil
		},
		Attempts:    retry.UnlimitedAttempts,
		Delay:       time.Second,
		MaxDelay:    5 * time.Minute,
		BackoffFunc: retry.DoubleDelay,
		Stop:        stop,
		Clock:       r.clock,
	})

	r.mutex.Lock()
	r.stopConnecting = nil
	defer r.mutex.Unlock()

	if connection != nil {
		r.connection = connection
		r.connectionReset = make(chan struct{})
		r.logger.Infof("forwarding connected %s -> %s", r.origin, r.target)
		_, err := r.hub.Publish(
			forwarder.ConnectedTopic,
			// NOTE: origin is filled in by the the central hub annotations.
			forwarder.OriginTarget{Target: r.target})
		if err != nil {
			r.logger.Errorf("%v", err)
		}
		return true
	}
	return false
}

func (r *remoteServer) loop() error {
	defer r.unsubscribe()

	var delay <-chan time.Time
	messageToSend := make(chan *params.PubSubMessage)
	messageSent := make(chan *params.PubSubMessage)
	go r.forwardMessages(messageToSend, messageSent)

	for {
		if r.connection == nil {
			// If we don't have a current connection, try to get one.
			if r.connect() {
				delay = nil
			} else {
				// Skip through the select to try to reconnect.
				delay = r.clock.After(time.Second)
			}
		}

		select {
		case <-r.tomb.Dying():
			r.logger.Debugf("worker shutting down")
			r.resetConnection()
			return tomb.ErrDying
		case <-r.data:
			// Has new data been pushed on?
			r.logger.Tracef("new messages")
		case <-delay:
			// If we failed to connect for whatever reason, this means we don't cycle
			// immediately.
			r.logger.Tracef("connect delay")
		}
		r.logger.Tracef("send pending messages")
		r.sendPendingMessages(messageToSend, messageSent)
	}
}

func (r *remoteServer) sendPendingMessages(messageToSend chan<- *params.PubSubMessage, messageSent <-chan *params.PubSubMessage) {
	for message := r.nextMessage(); message != nil; message = r.nextMessage() {
		select {
		case <-r.tomb.Dying():
			return
		case messageToSend <- message:
			// Just in case the worker dies while we are trying to send.
		}
		select {
		case <-r.tomb.Dying():
			// This will cause the main loop to iterate around, and close
			// the connection before returning.
			return
		case <-messageSent:
			// continue on to next
		}
	}
}

func (r *remoteServer) resetConnection() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	// If we have already been reset, just return
	if r.connection == nil {
		return
	}
	r.logger.Debugf("closing connection and clearing pending")
	r.connection.Close()
	r.connection = nil
	close(r.connectionReset)
	// Discard all pending messages.
	r.pending = deque.New()
	// Tell everyone what we have been disconnected.
	_, err := r.hub.Publish(
		forwarder.DisconnectedTopic,
		// NOTE: origin is filled in by the the central hub annotations.
		forwarder.OriginTarget{Target: r.target})
	if err != nil {
		r.logger.Errorf("%v", err)
	}
}

// forwardMessages is a goroutine whose sole purpose is to get messages off
// the messageToSend channel, try to send them over the API, and say when they
// are done with this message. This allows for the potential blocking call of
// `ForwardMessage`. If this does block for whatever reason and the worker is
// asked to shutdown, the main loop method is able to do so. That would cause
// the API connection to be closed, which would cause the `ForwardMessage` to
// be unblocked due to the error of the socket closing.
func (r *remoteServer) forwardMessages(messageToSend <-chan *params.PubSubMessage, messageSent chan<- *params.PubSubMessage) {
	var message *params.PubSubMessage
	for {
		select {
		case <-r.tomb.Dying():
			return
		case message = <-messageToSend:
		}
		r.mutex.Lock()
		conn := r.connection
		r.mutex.Unlock()

		r.logger.Tracef("forwarding %q to %s, data %v", message.Topic, r.target, message.Data)
		if conn != nil {
			err := conn.ForwardMessage(message)
			if err != nil {
				// Some problem sending, so log, close the connection, and try to reconnect.
				r.logger.Infof("unable to forward message, reconnecting... : %v", err)
				r.resetConnection()
			}
		}

		select {
		case <-r.tomb.Dying():
			return
		case messageSent <- message:
		}
	}
}

func (r *remoteServer) interruptConnecting() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.stopConnecting != nil {
		r.logger.Debugf("interrupting the pending connect loop")
		close(r.stopConnecting)
		r.stopConnecting = nil
	}
}

// Kill is part of the worker.Worker interface.
func (r *remoteServer) Kill() {
	r.tomb.Kill(nil)
	r.interruptConnecting()
}

// Wait is part of the worker.Worker interface.
func (r *remoteServer) Wait() error {
	return r.tomb.Wait()
}
