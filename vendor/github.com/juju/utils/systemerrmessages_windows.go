// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

// The following are strings/regex-es which match common Windows error messages
// that may be returned in case of failed calls to the system.
// Any extra leading/trailing regex-es are left to be added by the developer.
const (
	NoSuchUserErrRegexp = `No mapping between account names and security IDs was done\.`
	NoSuchFileErrRegexp = `The system cannot find the (file|path) specified\.`
	MkdirFailErrRegexp  = `mkdir .*` + NoSuchFileErrRegexp
)
