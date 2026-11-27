// Copyright 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit

import (
	"context"
	"fmt"
	"io"
	"strings"

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
	_, err := RunConfigureScriptContext(context.Background(), script, params)
	return err
}

// RunConfigureScriptContext runs the configuration script synchronously. It
// reports whether the remote shell confirmed that CleanupCommand succeeded,
// even if configuration failed. The context is checked before and after the
// SSH command because an in-flight command cannot be interrupted safely.
func RunConfigureScriptContext(ctx context.Context, script string, params ConfigureParams) (stagingCleaned bool, err error) {
	logger.Tracef(ctx, "running %d byte configuration script on %s", len(script), params.Host)

	const cleanupAck = "juju: staged files removed"
	var cleanupStaging string
	if params.CleanupCommand != "" {
		cleanupStaging = fmt.Sprintf(`
	if ( %s ) >/dev/null; then
		echo '%s'
	else
		echo "cleaning up staged files failed" >&2
		cleanupStatus=1
	fi`, params.CleanupCommand, cleanupAck)
	}

	// Bash parses the entire brace group before cat consumes the remaining
	// SSH input. This streams even a large setup script straight to disk.
	wrapper := `set -e
tmpfile=
cleanup() {
	local status=$?
	local cleanupStatus=0
	if [ -n "$tmpfile" ]; then
		rm -f -- "$tmpfile" || cleanupStatus=1
	fi` + cleanupStaging + `
	if [ "$status" -ne 0 ]; then
		exit "$status"
	fi
	exit "$cleanupStatus"
}
trap cleanup EXIT
tmpfile=$(mktemp)
{
	cat > "$tmpfile"
	/bin/bash "$tmpfile" >/dev/null
	exit
}
`
	client := params.Client
	if client == nil {
		client = ssh.DefaultClient
	}
	cmd := client.Command(params.Host, []string{"sudo", "/bin/bash", "-s"}, params.SSHOptions)
	cmd.Stdin = io.MultiReader(strings.NewReader(wrapper), strings.NewReader(script))
	cmd.Stderr = params.ProgressWriter
	// Only the trap writes to stdout. Setup continues to report progress
	// on stderr, and its exit status is independent of this acknowledgement.
	var ack strings.Builder
	if params.CleanupCommand != "" {
		cmd.Stdout = &ack
	}
	err = runCommand(ctx, cmd)
	return ack.String() == cleanupAck+"\n", err
}
