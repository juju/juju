// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package winrmprovisioner

import (
	"fmt"
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/utils/shell"
	"github.com/juju/utils/winrm"
)

// RemoveMachine returns nil if the removal process is done successfully
func RemoveMachine(args manual.RemoveMachineArgs) error {
	// we should be able to login to the windows machine using the user@host
	// values.
	client, err := loginAdministratorUser(args.WinrmClientAPI, args.User, args.Host)
	if err != nil {
		return errors.Trace(err)
	}

	// validate the machine has been provisioned with Juju.
	provisioned, err := checkProvisioned(args.Host, client)
	if err != nil {
		return errors.Annotatef(err, "error checking if provisioned")
	}
	if !provisioned {
		return errors.Errorf("machine not provisioned")
	}

	return runTearDownScript(args.WinrmClientAPI, args.Stdout, args.Stderr)
}

func loginAdministratorUser(client manual.WinrmClientAPI, user, host string) (manual.WinrmClientAPI, error) {
	if err := client.Ping(); err == nil {
		// We can connect, return back early
		return client, nil
	}

	// We can't ping, so let's try and connect in an insecure way, like how we
	// provision.
	logger.Debugf("Https client authentication is not enabled on the host %s with user %s", host, user)
	client, err := winrm.NewClient(winrm.ClientConfig{
		User:     user,
		Host:     host,
		Timeout:  25 * time.Second,
		Password: winrm.TTYGetPasswd,
		Secure:   false,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create a new http winrm client ")
	}

	logger.Infof("Trying http client as user %s on %s", host, user)
	if err = client.Ping(); err != nil {
		logger.Debugf("WinRM unsecure listener is not enabled on %s", host)
		return nil, errors.Annotatef(err, "cannot remove machine, because all winrm default connections failed")
	}

	// we can ping insecurely, let's try and use that setup.
	return client, nil
}

const removeChunk = `
Get-Service -DisplayName %[1]s | Stop-Service -Force
Get-Service -DisplayName %[1]s | Remove-Service

$provisionPath= [io.path]::Combine($ENV:APPDATA, '%[2]s', 'provision.ps1')
if (Test-Path $provisionPath) {
	Clear-Content $provisionPath
}
`

func runTearDownScript(client manual.WinrmClientAPI, stdout, stderr io.Writer) error {
	// if the file dosen't exist ,create it
	// if the file exists just clear/reset it
	script, err := shell.NewPSEncodedCommand(fmt.Sprintf(removeChunk, "jujud-machine-*", "Juju"))
	if err != nil {
		return err
	}
	if err = client.Run(script, stdout, stderr); err != nil {
		return errors.Trace(err)
	}
	return nil
}
