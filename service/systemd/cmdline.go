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

type commands struct {
	binary string
}

func (c commands) resolve(args string) string {
	binary := c.binary
	if binary == "" {
		binary = executable
	}
	return binary + " " + args
}

func (c commands) listAll() string {
	// We can't just use the same command as listRunning (with an extra
	// "--all" because it misses some inactive units.
	args := `list-unit-files --no-legend --no-page -t service` +
		` | grep -o -P '^\w[\S]*(?=\.service)'`
	return c.resolve(args)
}

func (c commands) listRunning() string {
	args := `--no-legend --no-page -t service` +
		` | grep -o -P '^\w[\S]*(?=\.service)'`
	return c.resolve(args)
}

func (c commands) activeStatus(name string) string {
	args := fmt.Sprintf("is-active %s.service || exit 0", name)
	return c.resolve(args)
}

func (c commands) loadStatus(name string) string {
	args := fmt.Sprintf("is-enabled %s.service || exit 0", name)
	return c.resolve(args)
}

func (c commands) start(name string) string {
	args := fmt.Sprintf("start %s.service", name)
	return c.resolve(args)
}

func (c commands) stop(name string) string {
	args := fmt.Sprintf("stop %s.service", name)
	return c.resolve(args)
}

func (c commands) link(name, dirname string) string {
	args := fmt.Sprintf("link %s/%s.service", dirname, name)
	return c.resolve(args)
}

func (c commands) enable(name string) string {
	args := fmt.Sprintf("enable %s.service", name)
	return c.resolve(args)
}

func (c commands) disable(name string) string {
	args := fmt.Sprintf("disable %s.service", name)
	return c.resolve(args)
}

func (c commands) reload() string {
	args := "daemon-reload"
	return c.resolve(args)
}

func (c commands) conf(name string) string {
	args := fmt.Sprintf("cat %s.service", name)
	return c.resolve(args)
}

func (c commands) writeFile(name, dirname string, data []byte) string {
	cmd := fmt.Sprintf("cat > %s/%s.service << 'EOF'\n%sEOF", dirname, name, data)
	return cmd
}

// Cmdline exposes the core operations of interacting with systemd units.
type Cmdline struct {
	commands commands
}

// TODO(ericsnow) Support more commands (Status, Start, Install, Conf, etc.).

// ListAll returns the names of all enabled systemd units.
func (cl Cmdline) ListAll() ([]string, error) {
	cmd := cl.commands.listAll()

	out, err := cl.runCommand(cmd, "List")
	if err != nil {
		return nil, errors.Trace(err)
	}
	out = strings.TrimSpace(out)

	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func (cl Cmdline) conf(name string) ([]byte, error) {
	cmd := cl.commands.conf(name)

	out, err := cl.runCommand(cmd, "get conf")
	if err != nil {
		return nil, errors.Trace(err)
	}
	out = strings.TrimSpace(out)

	return []byte(out), nil
}

const runCommandMsg = "%s failed (%s)"

func (Cmdline) runCommand(cmd, label string) (string, error) {
	resp, err := runCommands(exec.RunParams{
		Commands: cmd,
	})
	if err != nil {
		return "", errors.Annotatef(err, runCommandMsg, label, cmd)
	}
	out := string(resp.Stdout)

	if resp.Code != 0 {
		err := errors.Errorf(
			"error executing %q: %s",
			executable,
			strings.Replace(string(resp.Stderr), "\n", "; ", -1),
		)
		return out, errors.Annotatef(err, runCommandMsg, label, cmd)
	}
	return out, nil
}

var runCommands = func(args exec.RunParams) (*exec.ExecResponse, error) {
	return exec.RunCommands(args)
}
