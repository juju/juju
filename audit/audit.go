// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package audit records auditable events
package audit

import (
	"fmt"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.audit")

// Tagger represents anything that implements a Tag method.
type Tagger interface {
	Tag() string
}

// Audit records an auditable event against the tagged entity that performed the action.
func Audit(who Tagger, format string, args ...interface{}) {
	if who == nil {
		panic("who cannot be nil")
	}
	if who.Tag() == "" {
		panic("who cannot be blank")
	}
	logger.Logf(loggo.INFO, fmt.Sprintf("%s: %s", who.Tag(), format), args...)
}
