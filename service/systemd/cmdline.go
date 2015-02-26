// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/exec"
)

const executable = "/bin/systemctl"

type commands struct{}

// TODO(ericsnow) Always prepend `executable` to commands?

func (commands) listAll() string {
	// We can't just use the same command as listRunning (with an extra
	// "--all" because it misses some inactive units.
	args := `list-unit-files --no-legend --no-page -t service` +
		` | grep -o -P '^\w[\S]*(?=\.service)'`
	return args
}

func (commands) listRunning() string {
	args := `--no-legend --no-page -t service` +
		` | grep -o -P '^\w[\S]*(?=\.service)'`
	return args
}

func (commands) activeStatus(name string) string {
	args := fmt.Sprintf("is-active %s.service || exit 0", name)
	return args
}

func (commands) loadStatus(name string) string {
	args := fmt.Sprintf("is-enabled %s.service || exit 0", name)
	return args
}

func (commands) start(name string) string {
	args := fmt.Sprintf("start %s.service", name)
	return args
}

func (commands) stop(name string) string {
	args := fmt.Sprintf("stop %s.service", name)
	return args
}

func (commands) link(name, dirname string) string {
	args := fmt.Sprintf("link %s/%s.service", dirname, name)
	return args
}

func (commands) enable(name string) string {
	args := fmt.Sprintf("enable %s.service", name)
	return args
}

func (commands) disable(name string) string {
	args := fmt.Sprintf("disable %s.service", name)
	return args
}

func (commands) conf(name string) string {
	args := fmt.Sprintf("cat %s.service", name)
	return args
}

func (commands) writeConf(name, dirname, data string) string {
	args := fmt.Sprintf("cat >> %s/%s.service << 'EOF'\n%sEOF", dirname, name, data)
	return args
}

// Cmdline exposes the core operations of interacting with systemd units.
type Cmdline struct {
	commands commands
}

// TODO(ericsnow) Support more commands (Status, Start, Install, Conf, etc.).

// ListAll returns the names of all enabled systemd units.
func (cl Cmdline) ListAll() ([]string, error) {
	args := cl.commands.listAll()

	out, err := cl.runCommand(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return strings.Split(strings.TrimSpace(out), "\n"), nil
}

func (Cmdline) runCommand(cmd string) (string, error) {
	resp, err := exec.RunCommands(exec.RunParams{
		Commands: executable + " " + cmd,
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	out := string(resp.Stdout)

	if resp.Code != 0 {
		return out, errors.Errorf(
			"error executing %q: %s",
			executable,
			strings.Replace(string(resp.Stderr), "\n", "; ", -1),
		)
	}
	return out, nil
}
