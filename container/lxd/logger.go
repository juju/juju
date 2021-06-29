// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"bytes"
	"fmt"

	"github.com/juju/loggo/v2"
	lxdLogger "github.com/lxc/lxd/shared/logger"
)

// lxdLogProxy proxies LXD's log calls through the juju logger so we can get
// more info about what's going on.
type lxdLogProxy struct {
	logger loggo.Logger
}

func (p *lxdLogProxy) render(msg string, ctx []interface{}) string {
	var result bytes.Buffer
	result.WriteString(msg)
	if len(ctx) > 0 {
		result.WriteString(": ")
	}

	// This is sort of a hack, but it's enforced in the LXD code itself as
	// well. LXD's logging framework forces us to pass things as "string"
	// for one argument and then a "context object" as the next argument.
	// So, we do some basic rendering here to make it look slightly less
	// ugly.
	var key string
	for i, entry := range ctx {
		if i != 0 {
			result.WriteString(", ")
		}

		if key == "" {
			key, _ = entry.(string)
		} else {
			result.WriteString(key)
			result.WriteString(": ")
			result.WriteString(fmt.Sprintf("%s", entry))
			key = ""
		}
	}

	return result.String()
}

func (p *lxdLogProxy) Debug(msg string, ctx ...interface{}) {
	// NOTE(axw) the LXD client logs a lot of detail at
	// "debug" level, which is its highest level of logging.
	// We transform this to Trace, to avoid spamming our
	// logs with too much information.
	p.logger.Tracef(p.render(msg, ctx))
}

func (p *lxdLogProxy) Info(msg string, ctx ...interface{}) {
	p.logger.Infof(p.render(msg, ctx))
}

func (p *lxdLogProxy) Warn(msg string, ctx ...interface{}) {
	p.logger.Warningf(p.render(msg, ctx))
}

func (p *lxdLogProxy) Error(msg string, ctx ...interface{}) {
	p.logger.Errorf(p.render(msg, ctx))
}

func (p *lxdLogProxy) Crit(msg string, ctx ...interface{}) {
	p.logger.Criticalf(p.render(msg, ctx))
}

func init() {
	lxdLogger.Log = &lxdLogProxy{
		logger: loggo.GetLogger("lxd"),
	}
}
