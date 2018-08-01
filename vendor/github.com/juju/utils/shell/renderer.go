// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package shell

import (
	"runtime"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/filepath"
)

// A PathRenderer generates paths that are appropriate for a given
// shell environment.
type PathRenderer interface {
	filepath.Renderer

	// Quote generates a new string with quotation marks and relevant
	// escape/control characters properly escaped. The resulting string
	// is wrapped in quotation marks such that it will be treated as a
	// single string by the shell.
	Quote(str string) string

	// ExeSuffix returns the filename suffix for executable files.
	ExeSuffix() string
}

// Renderer provides all the functionality needed to generate shell-
// compatible paths and commands.
type Renderer interface {
	PathRenderer
	CommandRenderer
	OutputRenderer
}

// NewRenderer returns a Renderer for the given shell, OS, or distro name.
func NewRenderer(name string) (Renderer, error) {
	if name == "" {
		name = runtime.GOOS
	} else {
		name = strings.ToLower(name)
	}

	// Try known shell names first.
	switch name {
	case "bash":
		return &BashRenderer{}, nil
	case "ps", "powershell":
		return &PowershellRenderer{}, nil
	case "cmd", "batch", "bat":
		return &WinCmdRenderer{}, nil
	}

	// Fall back to operating systems.
	switch {
	case name == "windows":
		return &PowershellRenderer{}, nil
	case utils.OSIsUnix(name):
		return &BashRenderer{}, nil
	}

	// Finally try distros.
	switch name {
	case "ubuntu":
		return &BashRenderer{}, nil
	}

	return nil, errors.NotFoundf("renderer for %q", name)
}
