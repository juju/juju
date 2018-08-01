// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package shell

import (
	"strconv"
	"strings"
)

// OutputRenderer exposes the Renderer methods that relate to shell output.
//
// The methods all accept strings to identify their file descriptor
// arguments. While the interpretation of these values is up to the
// renderer, it will likely conform to the result of calling ResolveFD.
// If an FD arg is not recognized then no commands will be returned.
// Unless otherwise specified, the default file descriptor is stdout
// (FD 1). This applies to the empty string.
type OutputRenderer interface {
	// RedirectFD returns a shell command that redirects the src
	// file descriptor to the dst one.
	RedirectFD(dst, src string) []string

	// TODO(ericsnow) Add CopyFD and CreateFD?

	// TODO(ericsnow) Support passing the src FD as an arg?

	// RedirectOutput will cause all subsequent output from the shell
	// (or script) to go be appended to the given file. Only stdout is
	// redirected (use RedirectFD to redirect stderr or other FDs).
	//
	// The file should already exist (so a call to Touch may be
	// necessary before calling RedirectOutput). If the file should have
	// specific permissions or a specific owner then Chmod and Chown
	// should be called before calling RedirectOutput.
	RedirectOutput(filename string) []string

	// RedirectOutputReset will cause all subsequent output from the
	// shell (or script) to go be written to the given file. The file
	// will be reset (truncated to 0) before anything is written. Only
	// stdout is redirected (use RedirectFD to redirect stderr or
	// other FDs).
	//
	// The file should already exist (so a call to Touch may be
	// necessary before calling RedirectOutputReset). If the file should
	// have specific permissions or a specific owner then Chmod and
	// Chown should be called before calling RedirectOutputReset.
	RedirectOutputReset(filename string) []string
}

// ResolveFD converts the file descriptor name to the corresponding int.
// "stdout" and "out" match stdout (FD 1). "stderr" and "err" match
// stderr (FD 2), "stdin" and "in" match stdin (FD 0). All positive
// integers match. If there should be an upper bound then the caller
// should check it on the result. If the provided name is empty then
// the result defaults to stdout. If the name is not recognized then
// false is returned.
func ResolveFD(name string) (int, bool) {
	switch strings.ToLower(name) {
	case "stdout", "out", "":
		return 1, true
	case "stderr", "err":
		return 2, true
	case "stdin", "in":
		return 0, true
	default:
		fd, err := strconv.ParseUint(name, 10, 64)
		if err != nil {
			return -1, false
		}
		return int(fd), true
	}
}
