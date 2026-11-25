// Copyright 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.cloudinit.sshinit")

type ConfigureParams struct {
	// Host is the host to configure, in the format [user@]hostname.
	Host string

	// Client is the SSH client to connect with.
	// If Client is nil, ssh.DefaultClient will be used.
	Client ssh.Client

	// SSHOptions contains options for running the SSH command.
	SSHOptions *ssh.Options

	// Config is the cloudinit config to carry out.
	Config cloudinit.CloudConfig

	// CleanupCommand runs in the remote shell's EXIT trap after setup. It
	// avoids reconnecting after setup has changed the machine's SSH host keys.
	CleanupCommand string

	// ProgressWriter is an io.Writer to which progress will be written,
	// for realtime feedback.
	ProgressWriter io.Writer
}

// RunConfigureScript connects to the specified host over
// SSH, and executes the provided script which is expected
// to have been returned by cloudinit ConfigureScript.
func RunConfigureScript(script string, params ConfigureParams) error {
	return RunConfigureScriptContext(context.Background(), script, params)
}

// RunConfigureScriptContext runs the configuration script synchronously. The
// context is checked before and after the command; an already running command
// is not interrupted because the SSH library cannot safely force-close both
// backends.
func RunConfigureScriptContext(ctx context.Context, script string, params ConfigureParams) error {
	logger.Tracef(ctx, "running %d byte configuration script on %s", len(script), params.Host)

	cleanupTrap := `trap "rm -f $tmpfile" EXIT`
	if params.CleanupCommand != "" {
		cleanupTrap = fmt.Sprintf(`cleanup() {
	local status=$?
	rm -f "$tmpfile"
	if ! %s; then
		echo "cleaning up staged files failed" >&2
		if [ "$status" -eq 0 ]; then
			status=1
		fi
	fi
	return "$status"
}
trap cleanup EXIT`, params.CleanupCommand)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(`
set -e
tmpfile=$(mktemp)
` + cleanupTrap + `
cat > $tmpfile
/bin/bash $tmpfile
`))

	client := params.Client
	if client == nil {
		client = ssh.DefaultClient
	}

	// bash will read a byte at a time when consuming commands
	// from stdin. We avoid sending the entire script -- which
	// will be very large when uploading tools -- directly to
	// bash for this reason. Instead, run cat which will write
	// the script to disk, and then execute it from there.
	cmd := client.Command(params.Host, []string{
		"sudo", "/bin/bash", "-c",
		// The outer bash interprets the $(...), and executes
		// the decoded script in the nested bash. This avoids
		// linebreaks in the commandline, which the go.crypto-
		// based client has trouble with.
		fmt.Sprintf(
			`/bin/bash -c "$(echo %s | base64 -d)"`,
			utils.ShQuote(encoded),
		),
	}, params.SSHOptions)

	cmd.Stdin = strings.NewReader(script)
	cmd.Stderr = params.ProgressWriter
	return runCommand(ctx, cmd)
}
