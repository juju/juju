// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package observers

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/loggo"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

var logger = loggo.GetLogger("juju.apiserver")

// RequestNotifier serves as a sink for API server requests and
// responses. As of 2016-06-09, the ServerRequest method is called
// from github.com/juju/juju/rpc/server.go in the
// Conn::handleRequest(...) method.
type RequestNotifier struct {
	id    int64
	start time.Time

	mu   sync.Mutex
	tag_ string

	// count is incremented by calls to join, and deincremented
	// by calls to leave.
	count *int32
}

var globalCounter int64

func NewRequestNotifier(count *int32) *RequestNotifier {
	return &RequestNotifier{
		id:   atomic.AddInt64(&globalCounter, 1),
		tag_: "<unknown>",
		// TODO(fwereade): 2016-03-17 lp:1558657
		start: time.Now(),
		count: count,
	}
}

func (n *RequestNotifier) Login(tag string) {
	n.mu.Lock()
	n.tag_ = tag
	n.mu.Unlock()
}

func (n *RequestNotifier) tag() (tag string) {
	n.mu.Lock()
	tag = n.tag_
	n.mu.Unlock()
	return
}

func (n *RequestNotifier) ServerRequest(hdr *rpc.Header, body interface{}) {
	if hdr.Request.Type == "Pinger" && hdr.Request.Action == "Ping" {
		return
	}
	// TODO(rog) 2013-10-11 remove secrets from some requests.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if logger.IsTraceEnabled() {
		logger.Tracef("<- [%X] %s %s", n.id, n.tag(), jsoncodec.DumpRequest(hdr, body))
	} else {
		logger.Debugf("<- [%X] %s %s", n.id, n.tag(), jsoncodec.DumpRequest(hdr, "'params redacted'"))
	}
}

func (n *RequestNotifier) ServerReply(req rpc.Request, hdr *rpc.Header, body interface{}, timeSpent time.Duration) {
	if req.Type == "Pinger" && req.Action == "Ping" {
		return
	}
	// TODO(rog) 2013-10-11 remove secrets from some responses.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if logger.IsTraceEnabled() {
		logger.Tracef("-> [%X] %s %s", n.id, n.tag(), jsoncodec.DumpRequest(hdr, body))
	} else {
		logger.Debugf("-> [%X] %s %s %s %s[%q].%s", n.id, n.tag(), timeSpent, jsoncodec.DumpRequest(hdr, "'body redacted'"), req.Type, req.Id, req.Action)
	}
}

func (n *RequestNotifier) Join(req *http.Request) {
	active := atomic.AddInt32(n.count, 1)
	logger.Infof("[%X] API connection from %s, active connections: %d", n.id, req.RemoteAddr, active)
}

func (n *RequestNotifier) Leave() {
	active := atomic.AddInt32(n.count, -1)
	logger.Infof("[%X] %s API connection terminated after %v, active connections: %d", n.id, n.tag(), time.Since(n.start), active)
}

func (n *RequestNotifier) ClientRequest(hdr *rpc.Header, body interface{}) {
}

func (n *RequestNotifier) ClientReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
}
