// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"context"
	"net/http"
	"time"

	"github.com/juju/clock"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

// RequestLoggerConfig provides information needed for a
// RequestLogger to operate correctly.
type RequestLoggerConfig struct {
	// Clock is the clock to use for all time operations on this type.
	Clock clock.Clock

	// Logger is the log to use to write log statements.
	Logger logger.Logger
}

// RequestLogger serves as a sink for API server requests and
// responses.
type RequestLogger struct {
	BaseObserver

	clock      clock.Clock
	logger     logger.Logger
	connLogger logger.Logger
	pingLogger logger.Logger

	id                 uint64
	fd                 int
	websocketConnected time.Time
}

// NewRequestLogger returns a new RPCObserver.
func NewRequestLogger(ctx RequestLoggerConfig) *RequestLogger {
	// Ideally we should have a logging context so we can log into the correct
	// model rather than the api server for everything.
	return &RequestLogger{
		clock:      ctx.Clock,
		logger:     ctx.Logger,
		connLogger: ctx.Logger.Child("connection"),
		pingLogger: ctx.Logger.Child("ping"),
	}
}

// Login implements Observer.
func (n *RequestLogger) Login(ctx context.Context, entity names.Tag, model names.ModelTag, modelUUID model.UUID, fromController bool, userData string) {
	n.BaseObserver.Login(ctx, entity, model, modelUUID, fromController, userData)

	if !n.IsAgent() || n.FromController() {
		return
	}

	n.connLogger.Infof(ctx, "agent login: %s for %s", entity.String(), model.Id())
}

// Join implements Observer.
func (n *RequestLogger) Join(ctx context.Context, req *http.Request, connectionID uint64, fd int) {
	n.id = connectionID
	n.fd = fd
	n.websocketConnected = n.clock.Now()

	n.logger.Debugf(ctx,
		"[%X:%d] API connection from %s",
		n.id,
		n.fd,
		req.RemoteAddr,
	)
}

// Leave implements Observer.
func (n *RequestLogger) Leave(ctx context.Context) {
	if n.IsAgent() && !n.FromController() {
		// Don't log disconnections from the controller to the model.
		n.connLogger.Infof(ctx, "agent disconnected: %s for %s", n.AgentTagString(), n.ModelTag().Id())
	}

	// A leave event can be triggered without a login event, so we need to check
	// if the entity is an agent before logging.

	n.logger.Debugf(ctx,
		"[%X:%d] %s API connection terminated after %v",
		n.id,
		n.fd,
		n.AgentTagString(),
		n.clock.Now().Sub(n.websocketConnected),
	)
}

// RPCObserver implements Observer.
func (n *RequestLogger) RPCObserver() rpc.Observer {
	// A RPCObserver request can be called without a login event, so we need to
	// check if the entity is an agent before logging.

	return &rpcLogger{
		clock:      n.clock,
		logger:     n.logger,
		pingLogger: n.pingLogger,
		id:         n.id,
		tag:        n.AgentTagString(),
	}
}

// rpcLogger serves as a sink for RPC requests and responses.
type rpcLogger struct {
	clock        clock.Clock
	logger       logger.Logger
	pingLogger   logger.Logger
	id           uint64
	tag          string
	requestStart time.Time
}

// ServerRequest implements rpc.Observer.
func (n *rpcLogger) ServerRequest(ctx context.Context, hdr *rpc.Header, body interface{}) {
	// We know that if *at least* debug logging is not enabled, there will be
	// nothing to do here. Since this is a hot path, we can avoid the call to
	// DumpRequest below that would otherwise still be paid for every request.
	if !n.logger.IsLevelEnabled(logger.DEBUG) {
		return
	}

	n.requestStart = n.clock.Now()

	tracing := n.logger.IsLevelEnabled(logger.TRACE)

	if hdr.Request.Type == "Pinger" && hdr.Request.Action == "Ping" {
		if tracing {
			n.pingLogger.Tracef(ctx, "<- [%X] %s %s", n.id, n.tag, jsoncodec.DumpRequest(hdr, body))
		}
		return
	}

	// TODO(rog) 2013-10-11 remove secrets from some requests.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if tracing {
		n.logger.Tracef(ctx, "<- [%X] %s %s", n.id, n.tag, jsoncodec.DumpRequest(hdr, body))
	} else {
		n.logger.Debugf(ctx, "<- [%X] %s %s", n.id, n.tag, jsoncodec.DumpRequest(hdr, "'params redacted'"))
	}
}

// ServerReply implements rpc.Observer.
func (n *rpcLogger) ServerReply(ctx context.Context, req rpc.Request, hdr *rpc.Header, body interface{}) {
	// We know that if *at least* debug logging is not enabled, there will be
	// nothing to do here. Since this is a hot path, we can avoid the call to
	// DumpRequest below that would otherwise still be paid for every reply.
	if !n.logger.IsLevelEnabled(logger.DEBUG) {
		return
	}

	tracing := n.logger.IsLevelEnabled(logger.TRACE)

	if req.Type == "Pinger" && req.Action == "Ping" {
		if tracing {
			n.pingLogger.Tracef(ctx,
				"-> [%X] %s %s %s %s[%q].%s",
				n.id, n.tag, time.Since(n.requestStart), jsoncodec.DumpRequest(hdr, body), req.Type, req.Id, req.Action)
		}
		return
	}

	// TODO(rog) 2013-10-11 remove secrets from some responses.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if tracing {
		n.logger.Tracef(ctx,
			"-> [%X] %s %s %s %s[%q].%s",
			n.id, n.tag, time.Since(n.requestStart), jsoncodec.DumpRequest(hdr, body), req.Type, req.Id, req.Action)
	} else {
		n.logger.Debugf(ctx,
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
