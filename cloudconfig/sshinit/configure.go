// Copyright 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit

import (
	"io"
	"strings"

	"github.com/juju/loggo"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/utils/ssh"
)

var logger = loggo.GetLogger("juju.cloudinit.sshinit")

type ConfigureParams struct {
	// Host is the host to configure, in the format [user@]hostname.
	Host string

	// Client is the SSH client to connect with.
	// If Client is nil, ssh.DefaultClient will be used.
	Client ssh.Client

	// Config is the cloudinit config to carry out.
	Config cloudinit.CloudConfig

	// ProgressWriter is an io.Writer to which progress will be written,
	// for realtime feedback.
	ProgressWriter io.Writer

	// Series is the series of the machine on which the script will be carried out
	Series string
}

// Configure connects to the specified host over SSH,
// and executes a script that carries out cloud-config.
// This isn't actually used anywhere because everybody wants to add custom stuff
// in between getting the script and actually running it
// I really suggest deleting it
func Configure(params ConfigureParams) error {
	logger.Infof("Provisioning machine agent on %s", params.Host)
	script, err := params.Config.RenderScript()
	if err != nil {
		return err
	}
	return RunConfigureScript(script, params)
}

// RunConfigureScript connects to the specified host over
// SSH, and executes the provided script which is expected
// to have been returned by cloudinit ConfigureScript.
func RunConfigureScript(script string, params ConfigureParams) error {
	logger.Tracef("Running script on %s: %s", params.Host, script)
	cmd := ssh.Command(params.Host, []string{"sudo", "/bin/bash"}, nil)
	cmd.Stdin = strings.NewReader(script)
	cmd.Stderr = params.ProgressWriter
	return cmd.Run()
}
