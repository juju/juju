// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import "github.com/juju/juju/core/logger"

// loggerWrapper is an io.Writer() that forwards the messages to a
// logger.Logger. Unfortunately http takes a concrete stdlib log.Logger struct,
// and not an  interface, so we can't just proxy all of the log levels without
// inspecting the string content. For now, we just want to get the messages
// into the logs.
type loggerWrapper struct {
	logger logger.Logger
	level  logger.Level
}

func (w *loggerWrapper) Write(content []byte) (int, error) {
	w.logger.Logf(w.level, "%s", string(content))
	return len(content), nil
}
