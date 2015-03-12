// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"github.com/juju/errors"
	"gopkg.in/yaml.v1"
)

// UbuntuRenderer represents an Ubuntu specific script render
// type that is responsible for this particular OS. It implements
// the Renderer interface
type UbuntuRenderer struct{}

func (w *UbuntuRenderer) Mkdir(path string) []string {
	return []string{fmt.Sprintf(`mkdir -p %s`, utils.ShQuote(path))}
}

func (w *UbuntuRenderer) WriteFile(filename string, contents string, permission int) []string {
	quotedFilename := utils.ShQuote(filename)
	quotedContents := utils.ShQuote(contents)
	return []string{
		fmt.Sprintf("install -m %o /dev/null %s", permission, quotedFilename),
		fmt.Sprintf(`printf '%%s\n' %s > %s`, quotedContents, quotedFilename),
	}
}

func (w *UbuntuRenderer) FromSlash(filepath string) string {
	return filepath
}

func (w *UbuntuRenderer) PathJoin(filepath ...string) string {
	return path.Join(filepath...)
}

func (w *UbuntuRenderer) Render(conf CloudConfig) ([]byte, error) {
	data, err := yaml.Marshal(conf.getAttrs())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return append([]byte("#cloud-config\n"), data...), nil
}

// WindowsRenderer represents a Windows specific script render
// type that is responsible for this particular OS. It implements
// the Renderer interface
type WindowsRenderer struct{}

func (w *WindowsRenderer) Mkdir(path string) []string {
	return []string{fmt.Sprintf(`mkdir %s`, w.FromSlash(path))}
}

func (w *WindowsRenderer) WriteFile(filename string, contents string, permission int) []string {
	return []string{
		fmt.Sprintf("Set-Content '%s' @\"\n%s\n\"@", filename, contents),
	}
}

func (w *WindowsRenderer) PathJoin(filepath ...string) string {
	return strings.Join(filepath, `\`)
}

func (w *WindowsRenderer) FromSlash(path string) string {
	return strings.Replace(path, "/", `\`, -1)
}

func (w *WindowsRenderer) Render(conf CloudConfig) ([]byte, error) {
	winCmds := conf.RunCmds()
	var script []byte
	newline := "\r\n"
	header := "#ps1_sysnative\r\n"
	script = append(script, header...)
	for _, cmd := range winCmds {
		script = append(script, newline...)
		script = append(script, cmd...)

	}
	return script, nil
}
