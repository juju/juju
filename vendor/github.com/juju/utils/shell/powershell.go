// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package shell

import (
	"encoding/base64"
	"fmt"
	"os"

	"golang.org/x/text/encoding/unicode"

	"github.com/juju/errors"
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

// MkdirAll implements Renderer.
func (pr *PowershellRenderer) MkdirAll(dirname string) []string {
	return pr.Mkdir(dirname)
}

// ScriptFilename implements ScriptWriter.
func (pr *PowershellRenderer) ScriptFilename(name, dirname string) string {
	return pr.Join(dirname, name+".ps1")
}

// By default, winrm executes command usind cmd. Prefix the command we send over WinRM with powershell.exe.
// the powershell.exe it's a program that will execute the "%s" encoded command.
// A breakdown of the parameters:
//    -NonInteractive - prevent any prompts from stopping the execution of the scrips
//    -ExecutionPolicy - sets the execution policy for the current command, regardless of the default ExecutionPolicy on the system.
//    -EncodedCommand - allows us to run a base64 encoded script. This spares us from having to quote/escape shell special characters.
const psRemoteWrapper = "powershell.exe -Sta -NonInteractive -ExecutionPolicy RemoteSigned -EncodedCommand %s"

// newEncodedPSScript returns a UTF16-LE, base64 encoded script.
// The -EncodedCommand parameter expects this encoding for any base64 script we send over.
func newEncodedPSScript(script string) (string, error) {
	uni := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	encoded, err := uni.NewEncoder().String(script)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString([]byte(encoded)), nil
}

// NewPSEncodedCommand converts the given string to a UTF16-LE, base64 encoded string,
// suitable for execution using powershell.exe -EncodedCommand. This can be used on
// local systems, as well as remote systems via WinRM.
func NewPSEncodedCommand(script string) (string, error) {
	var err error
	script, err = newEncodedPSScript(script)
	if err != nil {
		return "", errors.Annotatef(err, "Cannot construct powershell command for remote execution")
	}

	return fmt.Sprintf(psRemoteWrapper, script), nil
}
