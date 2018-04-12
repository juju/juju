// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditlog

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

// Config holds parameters to control audit logging.
type Config struct {
	// Enabled determines whether API requests should be audited at
	// all.
	Enabled bool

	// CaptureAPIArgs says whether to capture API method args (command
	// line args will always be captured).
	CaptureAPIArgs bool

	// MaxSizeMB defines the maximum log file size.
	MaxSizeMB int

	// MaxBackups determines how many files back to keep.
	MaxBackups int

	// ExcludeMethods is a set of facade.method names that we
	// shouldn't consider to be interesting: if a conversation only
	// consists of these method calls we won't log it.
	ExcludeMethods set.Strings

	// Target is the AuditLog entries should be written to.
	Target AuditLog
}

// Validate checks the audit logging configuration.
func (cfg Config) Validate() error {
	if cfg.Enabled && cfg.Target == nil {
		return errors.NewNotValid(nil, "logging enabled but no target provided")
	}
	return nil
}
