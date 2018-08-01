// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package shell

import (
	"fmt"
	"os"

	"github.com/juju/utils"
)

// DumpFileOnErrorScript returns a bash script that
// may be used to dump the contents of the specified
// file to stderr when the shell exits with an error.
func DumpFileOnErrorScript(filename string) string {
	script := `
dump_file() {
    code=$?
    if [ $code -ne 0 -a -e %s ]; then
        cat %s >&2
    fi
    exit $code
}
trap dump_file EXIT
`[1:]
	filename = utils.ShQuote(filename)
	return fmt.Sprintf(script, filename, filename)
}

// A ScriptRenderer provides the functionality necessary to render a
// sequence of shell commands into the content of a shell script.
type ScriptRenderer interface {
	// RenderScript generates the content of a shell script for the
	// provided shell commands.
	RenderScript(commands []string) []byte
}

// A ScriptWriter provides the functionality necessarily to render and
// write a sequence of shell commands to a shell script that is ready
// to be run.
type ScriptWriter interface {
	ScriptRenderer

	// Chmod returns a shell command that sets the given file's
	// permissions. The result is equivalent to os.Chmod.
	Chmod(path string, perm os.FileMode) []string

	// WriteFile returns a shell command that writes the provided
	// content to a file. The command is functionally equivalent to
	// ioutil.WriteFile with permissions from the current umask.
	WriteFile(filename string, data []byte) []string

	// ScriptFilename generates a filename appropriate for a script
	// from the provided file and directory names.
	ScriptFilename(name, dirname string) string

	// ScriptPermissions returns the permissions appropriate for a script.
	ScriptPermissions() os.FileMode
}

// WriteScript returns a sequence of shell commands that write the
// provided shell commands to a file. The filename is composed from the
// given directory name and name, and the appropriate suffix for a shell
// script is applied. The script content is prefixed with any necessary
// content related to shell scripts (e.g. a shbang line). The file's
// permissions are set to those appropriate for a script (e.g. 0755).
func WriteScript(renderer ScriptWriter, name, dirname string, script []string) []string {
	filename := renderer.ScriptFilename(name, dirname)
	perm := renderer.ScriptPermissions()

	var commands []string

	data := renderer.RenderScript(script)
	commands = append(commands, renderer.WriteFile(filename, data)...)

	commands = append(commands, renderer.Chmod(filename, perm)...)

	return commands
}
