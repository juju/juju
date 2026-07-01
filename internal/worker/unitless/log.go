// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"

	"github.com/canonical/starform/starform"

	"github.com/juju/juju/core/logger"
)

type starformLogAdapter struct {
	logger logger.Logger
}

// NewStarformLogAdapter adapts a Juju logger to Starform's script logger.
func NewStarformLogAdapter(log logger.Logger) starform.Logger {
	return starformLogAdapter{logger: log}
}

func (l starformLogAdapter) Log(ctx context.Context, entry starform.LogEntry) {
	if l.logger == nil {
		return
	}
	switch entry.Level {
	case starform.DebugLevel:
		l.logger.Debugf(ctx, "scriptlet %s", entry.String())
	default:
		l.logger.Infof(ctx, "scriptlet %s", entry.String())
	}
}
