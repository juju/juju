// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"net/http"

	gc "gopkg.in/check.v1"
)

func NewHTTPHandlerForTest(
	newLogWriter NewLogWriteFunc,
	abort <-chan struct{},
	ratelimit *RateLimitConfig,
	metrics MetricsCollector,
	modelUUID string,
	makeChannel func() (chan struct{}, func()),
) http.Handler {
	return &logSinkHandler{
		newLogWriter:   newLogWriter,
		abort:          abort,
		ratelimit:      ratelimit,
		newStopChannel: makeChannel,
		metrics:        metrics,
		modelUUID:      modelUUID,
	}
}

func ReceiverStopped(c *gc.C, handler http.Handler) bool {
	h, ok := handler.(*logSinkHandler)
	c.Assert(ok, gc.Equals, true)
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.receiverStopped
}
