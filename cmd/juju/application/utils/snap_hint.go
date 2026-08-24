// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
)

// SnapEnv holds the snap environment variables that determine which paths the
// Juju snap is able to reach.
type SnapEnv struct {
	// Snap is $SNAP, empty when not running as a snap.
	Snap string
	// RealHome is $SNAP_REAL_HOME.
	RealHome string
	// Home is $HOME.
	Home string
	// UserData is $SNAP_USER_DATA.
	UserData string
	// UserCommon is $SNAP_USER_COMMON.
	UserCommon string
}

// SnapConfinementHint returns a hint message when running as a snap and path
// is outside the snap's reachable directories (HOME / SNAP_REAL_HOME /
// SNAP_USER_DATA / SNAP_USER_COMMON).
// Returns "" if not running as a snap, if the path is under a reachable root,
// or if path does not look like a filesystem path (no '/' separator).
func SnapConfinementHint(path string, env SnapEnv) string {
	if env.Snap == "" {
		return ""
	}
	// Only trigger for path-like arguments (contains a directory separator).
	if !strings.Contains(path, "/") {
		return ""
	}
	// A leading ~ is normally expanded by the shell before Juju sees the
	// argument. If it survives, the path is under the user's real home, which
	// the snap can reach, so a confinement hint would be wrong (and the
	// suggested cp would be a self-copy).
	if path == "~" || strings.HasPrefix(path, "~/") {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if isUnderRoot(abs, env.Home) || isUnderRoot(abs, env.RealHome) ||
		isUnderRoot(abs, env.UserData) || isUnderRoot(abs, env.UserCommon) {
		return ""
	}
	return fmt.Sprintf(
		"\n\nThe Juju snap is strictly confined and cannot access files outside your home\n"+
			"directory. Move the file into your home directory and try again, for example:\n\n"+
			"    cp %s ~/", shellQuote(path))
}

// shellQuote returns path quoted for safe use in a POSIX shell command. A path
// made only of characters the shell does not treat specially is returned
// unchanged, so the common suggestion stays readable; anything else is
// single-quoted. An embedded single quote is spliced in using the standard
// POSIX idiom of closing the quoting, escaping the quote, and reopening:
//
//	'\''
//
// Single quotes are used rather than double quotes because they also suppress
// $, backtick and backslash interpretation.
func shellQuote(path string) string {
	const safe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"0123456789@%+=:,./-_"
	if path == "" {
		return "''"
	}
	if strings.ContainsFunc(path, func(r rune) bool {
		return !strings.ContainsRune(safe, r)
	}) {
		return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
	}
	return path
}

// SnapConfinementHintFromEnv calls SnapConfinementHint using the process
// environment.
func SnapConfinementHintFromEnv(path string) string {
	return SnapConfinementHint(path, SnapEnv{
		Snap:       os.Getenv("SNAP"),
		RealHome:   os.Getenv("SNAP_REAL_HOME"),
		Home:       os.Getenv("HOME"),
		UserData:   os.Getenv("SNAP_USER_DATA"),
		UserCommon: os.Getenv("SNAP_USER_COMMON"),
	})
}

// isUnderRoot reports whether path is under root (root followed by a path
// separator).
func isUnderRoot(path, root string) bool {
	if root == "" {
		return false
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

// snapHintError appends a snap confinement hint to the message of the error it
// wraps. The hint is a suffix rather than a juju/errors annotation (which is a
// prefix) so that the underlying failure is still the first thing the user
// reads. Cause and Unwrap delegate to the wrapped error, so appending the hint
// does not change how callers classify the error.
type snapHintError struct {
	underlying error
	hint       string
}

// Error implements error.
func (e *snapHintError) Error() string {
	return e.underlying.Error() + e.hint
}

// Cause implements the causer interface used by errors.Cause.
func (e *snapHintError) Cause() error {
	return errors.Cause(e.underlying)
}

// Unwrap implements the interface used by errors.Is and errors.As.
func (e *snapHintError) Unwrap() error {
	return e.underlying
}

// AnnotateWithSnapHint returns err with a snap confinement hint appended to its
// message, when running as a snap and path is outside the directories the snap
// can reach. If no hint applies, err is returned unchanged. The error's cause
// and wrap chain are preserved either way.
func AnnotateWithSnapHint(err error, path string) error {
	if err == nil {
		return nil
	}
	hint := SnapConfinementHintFromEnv(path)
	if hint == "" {
		return err
	}
	return &snapHintError{underlying: err, hint: hint}
}
