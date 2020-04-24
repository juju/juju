// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package observer

import (
	"net/http"
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

// Hub defines the only method of the apiserver centralhub that
// the observer uses.
type Hub interface {
	Publish(topic string, data interface{}) (<-chan struct{}, error)
}

// RequestObserver serves as a sink for API server requests and
// responses.
type RequestObserver struct {
	clock              clock.Clock
	hub                Hub
	logger             loggo.Logger
	connLogger         loggo.Logger
	pingLogger         loggo.Logger
	apiConnectionCount func() int64

	// state represents information that's built up as methods on this
	// type are called. We segregate this to ensure it's clear what
	// information is transient in case we want to extract it
	// later. It's an anonymous struct so this doesn't leak outside
	// this type.
	state struct {
		id                 uint64
		websocketConnected time.Time
		tag                string
		model              string
		agent              bool
		fromController     bool
	}
}

// RequestObservercontext provides information needed for a
// RequestObserverContext to operate correctly.
type RequestObserverContext struct {

	// Clock is the clock to use for all time operations on this type.
	Clock clock.Clock

	// Hub refers to the pubsub Hub which will have connection
	// and disconnection events published.
	Hub Hub

	// Logger is the log to use to write log statements.
	Logger loggo.Logger
}

// NewRequestObserver returns a new RPCObserver.
func NewRequestObserver(ctx RequestObserverContext) *RequestObserver {
	// Ideally we should have a logging context so we can log into the correct
	// model rather than the api server for everything.
	module := ctx.Logger.Name()
	return &RequestObserver{
		clock:      ctx.Clock,
		hub:        ctx.Hub,
		logger:     ctx.Logger,
		connLogger: loggo.GetLogger(module + ".connection"),
		pingLogger: loggo.GetLogger(module + ".ping"),
	}
}

func (n *RequestObserver) isAgent(entity names.Tag) bool {
	switch entity.(type) {
	case names.UnitTag, names.MachineTag, names.ApplicationTag:
		return true
	default:
		return false
	}
}

// Login implements Observer.
func (n *RequestObserver) Login(entity names.Tag, model names.ModelTag, fromController bool, userData string) {
	n.state.tag = entity.String()
	n.state.fromController = fromController
	if n.isAgent(entity) {
		n.state.agent = true
		n.state.model = model.Id()
		// Don't log connections from the controller to the model.
		if !n.state.fromController {
			n.connLogger.Infof("agent login: %s for %s", n.state.tag, n.state.model)
		}
		n.hub.Publish(apiserver.ConnectTopic, apiserver.APIConnection{
			AgentTag:        n.state.tag,
			ControllerAgent: fromController,
			ModelUUID:       model.Id(),
			ConnectionID:    n.state.id,
			UserData:        userData,
		})
	}
}

// Join implements Observer.
func (n *RequestObserver) Join(req *http.Request, connectionID uint64) {
	n.state.id = connectionID
	n.state.websocketConnected = n.clock.Now()

	n.logger.Debugf(
		"[%X] API connection from %s",
		n.state.id,
		req.RemoteAddr,
	)
}

// Leave implements Observer.
func (n *RequestObserver) Leave() {
	if n.state.agent {
		// Don't log disconnections from the controller to the model.
		if !n.state.fromController {
			n.connLogger.Infof("agent disconnected: %s for %s", n.state.tag, n.state.model)
		}
		n.hub.Publish(apiserver.DisconnectTopic, apiserver.APIConnection{
			AgentTag:        n.state.tag,
			ControllerAgent: n.state.fromController,
			ModelUUID:       n.state.model,
			ConnectionID:    n.state.id,
		})
	}
	n.logger.Debugf(
		"[%X] %s API connection terminated after %v",
		n.state.id,
		n.state.tag,
		time.Since(n.state.websocketConnected),
	)
}

// RPCObserver implements Observer.
func (n *RequestObserver) RPCObserver() rpc.Observer {
	return &rpcObserver{
		clock:      n.clock,
		logger:     n.logger,
		pingLogger: n.pingLogger,
		id:         n.state.id,
		tag:        n.state.tag,
	}
}

// rpcObserver serves as a sink for RPC requests and responses.
type rpcObserver struct {
	clock        clock.Clock
	logger       loggo.Logger
	pingLogger   loggo.Logger
	id           uint64
	tag          string
	requestStart time.Time
}

// ServerRequest implements rpc.Observer.
func (n *rpcObserver) ServerRequest(hdr *rpc.Header, body interface{}) {
	n.requestStart = n.clock.Now()

	if hdr.Request.Type == "Pinger" && hdr.Request.Action == "Ping" {
		n.logRequestTrace(n.pingLogger, hdr, body)
		return
	}

	// TODO(rog) 2013-10-11 remove secrets from some requests.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if n.logger.IsTraceEnabled() {
		n.logRequestTrace(n.logger, hdr, body)
	} else {
		n.logger.Debugf("<- [%X] %s %s", n.id, n.tag, jsoncodec.DumpRequest(hdr, "'params redacted'"))
	}
}

// ServerReply implements rpc.Observer.
func (n *rpcObserver) ServerReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
	if req.Type == "Pinger" && req.Action == "Ping" {
		n.logReplyTrace(n.pingLogger, hdr, body)
		return
	}

	// TODO(rog) 2013-10-11 remove secrets from some responses.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if n.logger.IsTraceEnabled() {
		n.logReplyTrace(n.logger, hdr, body)
	} else {
		n.logger.Debugf(
			"-> [%X] %s %s %s %s[%q].%s",
			n.id,
			n.tag,
			time.Since(n.requestStart),
			jsoncodec.DumpRequest(hdr, "'body redacted'"),
			req.Type,
			req.Id,
			req.Action,
		)
	}
}

func (n *rpcObserver) logRequestTrace(logger loggo.Logger, hdr *rpc.Header, body interface{}) {
	n.logTrace(logger, "<-", hdr, body)
}

func (n *rpcObserver) logReplyTrace(logger loggo.Logger, hdr *rpc.Header, body interface{}) {
	n.logTrace(logger, "->", hdr, body)
}

func (n *rpcObserver) logTrace(logger loggo.Logger, prefix string, hdr *rpc.Header, body interface{}) {
	logger.Tracef("%s [%X] %s %s", prefix, n.id, n.tag, jsoncodec.DumpRequest(hdr, body))
}
