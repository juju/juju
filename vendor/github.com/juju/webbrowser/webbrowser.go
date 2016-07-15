// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package webbrowser

import (
	"errors"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Open opens a web browser at the given URL.
// If the OS is not recognized, an ErrNoBrowser is returned.
func Open(url *url.URL) error {
	var args []string
	if runtime.GOOS == "windows" {
		// Windows is special because the start command is built into cmd.exe
		// and hence requires the argument to be quoted.
		args = []string{"cmd", "/c", "start", winCmdQuote.Replace(url.String())}
	} else if b := browser[runtime.GOOS]; b != "" {
		args = []string{b, url.String()}
	} else {
		return ErrNoBrowser
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
	go cmd.Wait()
	return nil
}

// ErrNoBrowser is returned when a browser cannot be found for the current OS.
var ErrNoBrowser = errors.New("cannot find a browser to open the web page")

var browser = map[string]string{
	"linux":   "sensible-browser",
	"darwin":  "open",
	"freebsd": "xdg-open",
	"netbsd":  "xdg-open",
	"openbsd": "xdg-open",
}

// winCmdQuote can quote metacharacters special to the Windows
// cmd.exe command interpreter. It does that by inserting
// a '^' character before each metacharacter. Note that
// most of these cannot actually be produced by URL.String,
// but we include them for completeness.
var winCmdQuote = strings.NewReplacer(
	"&", "^&",
	"%", "^%",
	"(", "^(",
	")", "^)",
	"^", "^^",
	"<", "^<",
	">", "^>",
	"|", "^|",
)
