// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"fmt"

	"github.com/juju/utils/v3"
)

// progressFdEnvVar is the name of the environment variable set for
// the duration of the cloud-init script, identifying the file
// descriptor to which progress is logged. This is necessary so that
// we can redirect all non-progress related stderr away for interactive
// sessions (bootstrap, manual add-machine).
const progressFdEnvVar = "JUJU_PROGRESS_FD"

// InitProgressCmd will return a command to initialise progress
// reporting, sending messages to stderr. If LogProgressCmd is
// used in a script, InitProgressCmd MUST be executed beforehand.
//
// The returned commands are idempotent; this is important, to
// allow a script to be embedded in another with stderr redirected,
// in which case InitProgressCmd must precede the redirection.
func InitProgressCmd() string {
	// This command may be run by either bash or /bin/sh, the
	// latter of which does not support named file descriptors.
	// When running under /bin/sh we don't care about progress
	// logging, so we can allow it to go to FD 2.
	return fmt.Sprintf(
		`test -n "$%s" || `+
			`(exec {%s}>&2) 2>/dev/null && exec {%s}>&2 || `+
			`%s=2`,
		progressFdEnvVar,
		progressFdEnvVar,
		progressFdEnvVar,
		progressFdEnvVar,
	)
}

// LogProgressCmd will return a command to log the specified progress
// message to stderr; the resultant command should be added to the
// configuration as a runcmd or bootcmd as appropriate.
//
// If there are any uses of LogProgressCmd in a configuration, the
// configuration MUST precede the command with the result of
// InitProgressCmd.
func LogProgressCmd(format string, args ...interface{}) string {
	msg := utils.ShQuote(fmt.Sprintf(format, args...))
	return fmt.Sprintf("echo %s >&$%s", msg, progressFdEnvVar)
}
