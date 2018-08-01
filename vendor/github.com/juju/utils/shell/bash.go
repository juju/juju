// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package shell

import (
	"strings"
)

// BashRenderer is the shell renderer for bash.
type BashRenderer struct {
	unixRenderer
}

// Render implements ScriptWriter.
func (*BashRenderer) RenderScript(commands []string) []byte {
	commands = append([]string{"#!/usr/bin/env bash", ""}, commands...)
	return []byte(strings.Join(commands, "\n"))
}
