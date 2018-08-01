// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package shell

import (
	"bytes"
	"fmt"
	"os"

	"github.com/juju/utils"
)

// WinCmdRenderer is a shell renderer for Windows cmd.exe.
type WinCmdRenderer struct {
	windowsRenderer
}

// Quote implements Renderer.
func (wcr *WinCmdRenderer) Quote(str string) string {
	return utils.WinCmdQuote(str)
}

// Chmod implements Renderer.
func (wcr *WinCmdRenderer) Chmod(path string, perm os.FileMode) []string {
	// TODO(ericsnow) Is this necessary? Should we use icacls?
	return nil
}

// WriteFile implements Renderer.
func (wcr *WinCmdRenderer) WriteFile(filename string, data []byte) []string {
	filename = wcr.Quote(filename)
	var commands []string
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		cmd := fmt.Sprintf(">>%s @echo %s", filename, line)
		commands = append(commands, cmd)
	}
	return commands
}

// MkDir implements Renderer.
func (wcr *WinCmdRenderer) Mkdir(dirname string) []string {
	dirname = wcr.Quote(dirname)
	return []string{
		fmt.Sprintf(`mkdir %s`, wcr.FromSlash(dirname)),
	}
}

// MkDirAll implements Renderer.
func (wcr *WinCmdRenderer) MkdirAll(dirname string) []string {
	dirname = wcr.Quote(dirname)
	// TODO(ericsnow) Wrap in "setlocal enableextensions...endlocal"?
	return []string{
		fmt.Sprintf(`mkdir %s`, wcr.FromSlash(dirname)),
	}
}

// ScriptFilename implements ScriptWriter.
func (wcr *WinCmdRenderer) ScriptFilename(name, dirname string) string {
	return wcr.Join(dirname, name+".bat")
}
