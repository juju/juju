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

	// go vet thinks that the third parameter of aliasFn should be a string constant.
	// fixes error message: "audit/audit.go:30: constant 3 not a string in call to Logf"
	var aliasFn func(loggo.Level, string, ...interface{}) = logger.Logf

	// Logf is called directly, rather than Infof so that the caller of Audit is
	// recorded, not Audit itself.
	aliasFn(loggo.INFO, fmt.Sprintf("%s: %s", user.Tag(), format), args...)
}
