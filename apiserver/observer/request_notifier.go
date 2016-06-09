// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package observer

import (
	"net/http"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

// RequestNotifier serves as a sink for API server requests and
// responses.
type RequestNotifier struct {
	clock              clock.Clock
	logger             loggo.Logger
	apiConnectionCount func() int64
	id                 int64

	// state represents information that's built up as methods on this
	// type are called. We segregate this to ensure it's clear what
	// information is transient in case we want to extract it
	// later. It's an anonymous struct so this doesn't leak outside
	// this type.
	state struct {
		websocketConnected time.Time
		requestStart       time.Time
		tag                string
	}
}

// RequestNotifierContext provides information needed for a
// RequestNotifier to operate correctly.
type RequestNotifierContext struct {

	// Clock is the clock to use for all time operations on this type.
	Clock clock.Clock

	// Logger is the log to use to write log statements.
	Logger loggo.Logger
}

// NewRequestNotifier returns a new RequestNotifier.
func NewRequestNotifier(ctx RequestNotifierContext, id int64) *RequestNotifier {
	return &RequestNotifier{
		clock:  ctx.Clock,
		logger: ctx.Logger,
		id:     id,
	}
}

// Login implements Observer.
func (n *RequestNotifier) Login(tag string) {
	n.state.tag = tag
}

// ServerReques timplements Observer.
func (n *RequestNotifier) ServerRequest(hdr *rpc.Header, body interface{}) {
	n.state.requestStart = n.clock.Now()
	if hdr.Request.Type == "Pinger" && hdr.Request.Action == "Ping" {
		return
	}
	// TODO(rog) 2013-10-11 remove secrets from some requests.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if n.logger.IsTraceEnabled() {
		n.logger.Tracef("<- [%X] %s %s", n.id, n.state.tag, jsoncodec.DumpRequest(hdr, body))
	} else {
		n.logger.Debugf("<- [%X] %s %s", n.id, n.state.tag, jsoncodec.DumpRequest(hdr, "'params redacted'"))
	}
}

// ServerReply implements Observer.
func (n *RequestNotifier) ServerReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
	if req.Type == "Pinger" && req.Action == "Ping" {
		return
	}
	// TODO(rog) 2013-10-11 remove secrets from some responses.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if n.logger.IsTraceEnabled() {
		n.logger.Tracef("-> [%X] %s %s", n.id, n.state.tag, jsoncodec.DumpRequest(hdr, body))
	} else {
		n.logger.Debugf(
			"-> [%X] %s %s %s %s[%q].%s",
			n.id,
			n.state.tag,
			time.Since(n.state.requestStart),
			jsoncodec.DumpRequest(hdr, "'body redacted'"),
			req.Type,
			req.Id,
			req.Action,
		)
	}
}

// Join implements Observer.
func (n *RequestNotifier) Join(req *http.Request) {
	n.state.websocketConnected = n.clock.Now()
	n.logger.Infof(
		"[%X] API connection from %s",
		n.id,
		req.RemoteAddr,
	)
}

// Leave implements Observer.
func (n *RequestNotifier) Leave() {
	n.logger.Infof(
		"[%X] %s API connection terminated after %v",
		n.id,
		n.state.tag,
		time.Since(n.state.websocketConnected),
	)
}
