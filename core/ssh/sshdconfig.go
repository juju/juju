// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"bufio"
	"io"
	"os"
	"strings"

	"github.com/juju/juju/internal/errors"
)

// DefaultSSHDPort is the port sshd listens on when no Port directive is set in
// the sshd_config file.
const DefaultSSHDPort = "22"

// DefaultSSHDConfigPaths are the well-known locations of the sshd_config file,
// tried in order.
var DefaultSSHDConfigPaths = []string{
	"/etc/ssh/sshd_config",
	"/usr/share/openssh/sshd_config",
}

// SSHDConfig represents the parsed contents of an sshd_config file that are of
// interest to Juju. It is a read-only view: parse it once from a file or an
// io.Reader, then query it.
type SSHDConfig struct {
	// port is the port sshd listens on, defaulting to DefaultSSHDPort when no
	// Port directive is present.
	port string
}

// ParseSSHDConfig reads sshd_config content from r and returns the parsed
// configuration. It returns an error only if the underlying reader fails; a
// missing Port directive is not an error and yields DefaultSSHDPort.
func ParseSSHDConfig(r io.Reader) (*SSHDConfig, error) {
	cfg := &SSHDConfig{port: DefaultSSHDPort}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "Port") {
			fields := strings.Fields(line)
			if len(fields) == 2 {
				cfg.port = fields[1]
			}
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.Capture(err)
	}
	return cfg, nil
}

// OpenSSHDConfig opens the sshd_config file at filePath and returns the parsed
// configuration. It returns an error if the file cannot be opened or read.
func OpenSSHDConfig(filePath string) (*SSHDConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, errors.Capture(err)
	}
	defer func() { _ = file.Close() }()

	return ParseSSHDConfig(file)
}

// Port returns the port sshd listens on, or DefaultSSHDPort if no Port
// directive was present.
func (c *SSHDConfig) Port() string {
	return c.port
}
