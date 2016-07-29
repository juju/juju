// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

// +build !windows

package utils

// The following are strings/regex-es which match common Unix error messages
// that may be returned in case of failed calls to the system.
// Any extra leading/trailing regex-es are left to be added by the developer.
const (
	NoSuchUserErrRegexp = `user: unknown user [a-z0-9_-]*`
	NoSuchFileErrRegexp = `no such file or directory`
	MkdirFailErrRegexp  = `.* not a directory`
)
