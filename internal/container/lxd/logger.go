// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"bytes"
	"context"
	"fmt"

	lxdLogger "github.com/canonical/lxd/shared/logger"

	corelogger "github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
)

// lxdLogProxy proxies LXD's log calls through the juju logger.
type lxdLogProxy struct {
	logger corelogger.Logger
}

func (p *lxdLogProxy) render(msg string, ctx []lxdLogger.Ctx) string {
	var result bytes.Buffer
	result.WriteString(msg)
	if len(ctx) > 0 {
		result.WriteString(": ")
	}

	for _, c := range ctx {
		var afterFirst bool
		for k, v := range c {
			if afterFirst {
				result.WriteString(", ")
			}
			afterFirst = true

			result.WriteString(k)
			result.WriteString(": ")
			result.WriteString(fmt.Sprintf("%v", v))
		}
	}

	return result.String()
}

func (p *lxdLogProxy) Trace(msg string, ctx ...lxdLogger.Ctx) {
	p.logger.Tracef(context.TODO(), p.render(msg, ctx))
}

func (p *lxdLogProxy) Debug(msg string, ctx ...lxdLogger.Ctx) {
	// NOTE(axw) the LXD client logs a lot of detail at
	// "debug" level, which is its highest level of logging.
	// We transform this to Trace, to avoid spamming our
	// logs with too much information.
	p.logger.Tracef(context.TODO(), p.render(msg, ctx))
}

func (p *lxdLogProxy) Info(msg string, ctx ...lxdLogger.Ctx) {
	p.logger.Infof(context.TODO(), p.render(msg, ctx))
}

func (p *lxdLogProxy) Warn(msg string, ctx ...lxdLogger.Ctx) {
	p.logger.Warningf(context.TODO(), p.render(msg, ctx))
}

func (p *lxdLogProxy) Error(msg string, ctx ...lxdLogger.Ctx) {
	p.logger.Errorf(context.TODO(), p.render(msg, ctx))
}

func (p *lxdLogProxy) Crit(msg string, ctx ...lxdLogger.Ctx) {
	p.logger.Criticalf(context.TODO(), p.render(msg, ctx))
}

func (p *lxdLogProxy) Fatal(msg string, ctx ...lxdLogger.Ctx) {
	p.logger.Criticalf(context.TODO(), "Fatal: %s", p.render(msg, ctx))
}

func (p *lxdLogProxy) Panic(msg string, ctx ...lxdLogger.Ctx) {
	p.logger.Criticalf(context.TODO(), "Panic: %s", p.render(msg, ctx))
}

func (p *lxdLogProxy) AddContext(_ lxdLogger.Ctx) lxdLogger.Logger {
	return p
}

func init() {
	lxdLogger.Log = &lxdLogProxy{
		logger: internallogger.GetLogger("lxd"),
	}
}
