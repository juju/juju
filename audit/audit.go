// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package audit records auditable events
package audit

import (
	"fmt"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("audit")

// Tagger represents anything that implements a Tag method.
type Tagger interface {
	Tag() string
}

// Audit records an auditable event against the tagged entity that performed the action.
func Audit(user Tagger, format string, args ...interface{}) {
	if user == nil {
		panic("user cannot be nil")
	}
	if user.Tag() == "" {
		panic("user tag cannot be blank")
	}
	// Logf is called directly, rather than Infof so that the caller of Audit is
	// recorded, not Audit itself.
	logger.Logf(loggo.INFO, fmt.Sprintf("%s: %s", user.Tag(), format), args...)
}
