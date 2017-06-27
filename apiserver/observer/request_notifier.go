// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package observer

import (
	"net/http"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

// RequestObserver serves as a sink for API server requests and
// responses.
type RequestObserver struct {
	clock              clock.Clock
	logger             loggo.Logger
	connLogger         loggo.Logger
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
	}
}

// RequestObservercontext provides information needed for a
// RequestObserverContext to operate correctly.
type RequestObserverContext struct {

	// Clock is the clock to use for all time operations on this type.
	Clock clock.Clock

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
		logger:     ctx.Logger,
		connLogger: loggo.GetLogger(module + ".connection"),
	}
}

func (n *RequestObserver) isAgent(entity names.Tag) bool {
	switch entity.(type) {
	case names.UnitTag, names.MachineTag:
		return true
	default:
		return false
	}
}

// Login implements Observer.
func (n *RequestObserver) Login(entity names.Tag, model names.ModelTag, fromController bool, _ string) {
	n.state.tag = entity.String()
	// Don't log connections from the controller to the model.
	if n.isAgent(entity) && !fromController {
		n.state.agent = true
		n.state.model = model.Id()
		n.connLogger.Infof("agent login: %s for %s", n.state.tag, n.state.model)
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
		n.connLogger.Infof("agent disconnected: %s for %s", n.state.tag, n.state.model)
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
		clock:  n.clock,
		logger: n.logger,
		id:     n.state.id,
		tag:    n.state.tag,
	}
}

// rpcObserver serves as a sink for RPC requests and responses.
type rpcObserver struct {
	clock        clock.Clock
	logger       loggo.Logger
	id           uint64
	tag          string
	requestStart time.Time
}

// ServerReques timplements rpc.Observer.
func (n *rpcObserver) ServerRequest(hdr *rpc.Header, body interface{}) {
	n.requestStart = n.clock.Now()

	if hdr.Request.Type == "Pinger" && hdr.Request.Action == "Ping" {
		return
	}
	// TODO(rog) 2013-10-11 remove secrets from some requests.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if n.logger.IsTraceEnabled() {
		n.logger.Tracef("<- [%X] %s %s", n.id, n.tag, jsoncodec.DumpRequest(hdr, body))
	} else {
		n.logger.Debugf("<- [%X] %s %s", n.id, n.tag, jsoncodec.DumpRequest(hdr, "'params redacted'"))
	}
}

// ServerReply implements rpc.Observer.
func (n *rpcObserver) ServerReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
	if req.Type == "Pinger" && req.Action == "Ping" {
		return
	}

	// TODO(rog) 2013-10-11 remove secrets from some responses.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if n.logger.IsTraceEnabled() {
		n.logger.Tracef("-> [%X] %s %s", n.id, n.tag, jsoncodec.DumpRequest(hdr, body))
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
