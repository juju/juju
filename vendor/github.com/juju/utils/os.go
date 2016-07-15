// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

// These are the names of the operating systems recognized by Go.
const (
	OSWindows   = "windows"
	OSDarwin    = "darwin"
	OSDragonfly = "dragonfly"
	OSFreebsd   = "freebsd"
	OSLinux     = "linux"
	OSNacl      = "nacl"
	OSNetbsd    = "netbsd"
	OSOpenbsd   = "openbsd"
	OSSolaris   = "solaris"
)

// OSUnix is the list of unix-like operating systems recognized by Go.
// See http://golang.org/src/path/filepath/path_unix.go.
var OSUnix = []string{
	OSDarwin,
	OSDragonfly,
	OSFreebsd,
	OSLinux,
	OSNacl,
	OSNetbsd,
	OSOpenbsd,
	OSSolaris,
}

// OSIsUnix determines whether or not the given OS name is one of the
// unix-like operating systems recognized by Go.
func OSIsUnix(os string) bool {
	for _, goos := range OSUnix {
		if os == goos {
			return true
		}
	}
	return false
}
