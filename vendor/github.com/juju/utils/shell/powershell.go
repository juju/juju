// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package shell

import (
	"fmt"
	"os"

	"github.com/juju/utils"
)

// PowershellRenderer is a shell renderer for Windows Powershell.
type PowershellRenderer struct {
	windowsRenderer
}

// Quote implements Renderer.
func (pr *PowershellRenderer) Quote(str string) string {
	return utils.WinPSQuote(str)
}

// Chmod implements Renderer.
func (pr *PowershellRenderer) Chmod(path string, perm os.FileMode) []string {
	// TODO(ericsnow) Is this necessary? Should we use Set-Acl?
	return nil
}

// WriteFile implements Renderer.
func (pr *PowershellRenderer) WriteFile(filename string, data []byte) []string {
	filename = pr.Quote(filename)
	return []string{
		fmt.Sprintf("Set-Content %s @\"\n%s\n\"@", filename, data),
	}
}

// MkDir implements Renderer.
func (pr *PowershellRenderer) Mkdir(dirname string) []string {
	dirname = pr.FromSlash(dirname)
	return []string{
		fmt.Sprintf(`mkdir %s`, pr.Quote(dirname)),
	}
}

// MkDirAll implements Renderer.
func (pr *PowershellRenderer) MkdirAll(dirname string) []string {
	return pr.Mkdir(dirname)
}

// ScriptFilename implements ScriptWriter.
func (pr *PowershellRenderer) ScriptFilename(name, dirname string) string {
	return pr.Join(dirname, name+".ps1")
}
