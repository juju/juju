// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"fmt"

	"launchpad.net/juju-core/utils"
)

// progressFd is the file descriptor to which progress is logged.
// This is necessary so that we can redirect all non-progress
// related stderr away.
//
// Note, from the Bash manual:
//   "Redirections using file descriptors greater than 9 should be
//   used with care, as they may conflict with file descriptors the
//   shell uses internally."
const progressFd = 9

// InitProgressCmd will return a command to initialise progress
// reporting, sending messages to stderr. If LogProgressCmd is
// used in a script, InitProgressCmd MUST be executed beforehand.
//
// The returned command is idempotent; this is important, to
// allow a script to be embedded in another with stderr redirected,
// in which case InitProgressCmd must precede the redirection.
func InitProgressCmd() string {
	return fmt.Sprintf("test -e /proc/self/fd/%d || exec %d>&2", progressFd, progressFd)
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
	return fmt.Sprintf("echo %s >&%d", msg, progressFd)
}
