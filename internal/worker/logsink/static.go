// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	corelogger "github.com/juju/juju/core/logger"
)

type staticLogRouter struct {
	logSink corelogger.LogSink
}

func (s staticLogRouter) LogSink() corelogger.LogSink {
	return s.logSink
}

// StaticLogRouter returns a LogRouter that always delegates to the supplied
// LogSink. It is intended for controller-local logging where the API-backed
// log router is not yet available.
func StaticLogRouter(logSink corelogger.LogSink) LogRouter {
	return staticLogRouter{logSink: logSink}
}
