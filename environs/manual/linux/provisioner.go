// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package linux

import (
	"io"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/manual/common"

	"github.com/juju/loggo"
)

var (
	logger = loggo.GetLogger("juju.environs.manual.linux")
)

const Scope = "ssh"

type provision struct {
	host string
	user string

	client common.ProvisioningClientAPI

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	authorizedKeys string

	*params.UpdateBehavior
}

// NewProvisioner returns a new linux provision
func NewProvisioner(args common.ProvisionMachineArgs) *provision {
	linux := &provision{
		host:           args.Host,
		user:           args.User,
		client:         args.Client,
		stdin:          args.Stdin,
		stdout:         args.Stdout,
		stderr:         args.Stderr,
		authorizedKeys: args.AuthorizedKeys,
	}

	linux.UpdateBehavior = new(params.UpdateBehavior)
	linux.UpdateBehavior.EnableOSUpgrade = args.EnableOSUpgrade
	linux.UpdateBehavior.EnableOSRefreshUpdate = args.EnableOSRefreshUpdate

	return linux
}

// Provision returns a new machineId and nil if the provision process is done successfully
// The func will manual provision a linux machine using as it's default protocol SSH
func (p provision) Provision() (machineId string, err error) {
	if err = p.init(); err != nil {
		return "", err
	}

	machineParams, err := p.gatherMachineParams()
	if err != nil {
		return "", err
	}

	// Inform Juju that the machine exists.
	machineId, err = common.RecordMachineInState(p.client, *machineParams)
	if err != nil {
		return "", err
	}

	provisioningScript, err := p.client.ProvisioningScript(params.ProvisioningScriptParams{
		MachineId: machineId,
		Nonce:     machineParams.Nonce,
		DisablePackageCommands: !p.EnableOSRefreshUpdate && !p.EnableOSUpgrade,
	})

	if err != nil {
		logger.Errorf("cannot obtain provisioning script")
		return "", err
	}

	// Finally, provision the machine agent.
	err = runProvisionScript(provisioningScript, p.host, p.stderr)
	if err != nil {
		return machineId, err
	}

	logger.Infof("Provisioned machine %v", machineId)
	return machineId, nil
}

func (p provision) init() error {
	// Create the "ubuntu" user and initialise passwordless sudo. We populate
	// the ubuntu user's authorized_keys file with the public keys in the current
	// user's ~/.ssh directory. The authenticationworker will later update the
	// ubuntu user's authorized_keys.
	return InitUbuntuUser(p.host, p.user, p.authorizedKeys, p.stdin, p.stdout)
}

func (p provision) gatherMachineParams() (*params.AddMachineParams, error) {
	return gatherMachineParams(p.host)
}