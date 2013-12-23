// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
	"launchpad.net/juju-core/worker/localstorage"
)

const BootstrapInstanceId = instance.Id(manualInstancePrefix)

// LocalStorageEnviron is an Environ where the bootstrap node
// manages its own local storage.
type LocalStorageEnviron interface {
	environs.Environ
	localstorage.LocalStorageConfig
}

type BootstrapArgs struct {
	Host                    string
	DataDir                 string
	Environ                 LocalStorageEnviron
	PossibleTools           tools.List
	Series                  string
	HardwareCharacteristics *instance.HardwareCharacteristics
}

func errMachineIdInvalid(machineId string) error {
	return fmt.Errorf("%q is not a valid machine ID", machineId)
}

// NewManualBootstrapEnviron wraps a LocalStorageEnviron with another which
// overrides the Bootstrap method; when Bootstrap is invoked, the specified
// host will be manually bootstrapped.
//
// InitUbuntuUser is expected to have been executed successfully against
// the host being bootstrapped.
func Bootstrap(args BootstrapArgs) (err error) {
	if args.Host == "" {
		return errors.New("host argument is empty")
	}
	if args.Environ == nil {
		return errors.New("environ argument is nil")
	}
	if args.DataDir == "" {
		return errors.New("data-dir argument is empty")
	}
	if args.Series == "" {
		return errors.New("series argument is empty")
	}
	if args.HardwareCharacteristics == nil {
		return errors.New("hardware characteristics argument is empty")
	}

	sshHost := "ubuntu@" + args.Host
	provisioned, err := checkProvisioned(sshHost)
	if err != nil {
		return fmt.Errorf("failed to check provisioned status: %v", err)
	}
	if provisioned {
		return ErrProvisioned
	}

	// Filter tools based on detected series/arch.
	logger.Infof("Filtering possible tools: %v", args.PossibleTools)
	possibleTools, err := args.PossibleTools.Match(tools.Filter{
		Arch:   *args.HardwareCharacteristics.Arch,
		Series: args.Series,
	})
	if err != nil {
		return err
	}

	// Store the state file. If provisioning fails, we'll remove the file.
	logger.Infof("Saving bootstrap state file to bootstrap storage")
	bootstrapStorage := args.Environ.Storage()
	err = bootstrap.SaveState(
		bootstrapStorage,
		&bootstrap.BootstrapState{
			StateInstances:  []instance.Id{BootstrapInstanceId},
			Characteristics: []instance.HardwareCharacteristics{*args.HardwareCharacteristics},
		},
	)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			logger.Errorf("bootstrapping failed, removing state file: %v", err)
			bootstrapStorage.Remove(bootstrap.StateFile)
		}
	}()

	// Get a file:// scheme tools URL for the tools, which will have been
	// copied to the remote machine's storage directory.
	tools := *possibleTools[0]
	storageDir := args.Environ.StorageDir()
	toolsStorageName := envtools.StorageName(tools.Version)
	tools.URL = fmt.Sprintf("file://%s/%s", storageDir, toolsStorageName)

	// Add the local storage configuration.
	agentEnv, err := localstorage.StoreConfig(args.Environ)
	if err != nil {
		return err
	}

	// Finally, provision the machine agent.
	stateFileURL := fmt.Sprintf("file://%s/%s", storageDir, bootstrap.StateFile)
	mcfg := environs.NewBootstrapMachineConfig(stateFileURL)
	if args.DataDir != "" {
		mcfg.DataDir = args.DataDir
	}
	mcfg.Tools = &tools
	err = environs.FinishMachineConfig(mcfg, args.Environ.Config(), constraints.Value{})
	if err != nil {
		return err
	}
	for k, v := range agentEnv {
		mcfg.AgentEnvironment[k] = v
	}
	return provisionMachineAgent(sshHost, mcfg)
}

// InitUbuntuUser adds the ubuntu user if it doesn't
// already exist, and updates its ~/.ssh/authorized_keys.
//
// authorizedKeys may be empty, in which case the file
// will be created and left empty.
func InitUbuntuUser(host, login, authorizedKeys string) error {
	logger.Infof("initialising %q, user %q", host, login)

	// To avoid unnecessary prompting for the specified login,
	// initUbuntuUser will first attempt to ssh to the machine
	// as "ubuntu" with password authentication disabled, and
	// ensure that it can use sudo without a password.
	//
	// Note that we explicitly do not allocate a PTY, so we
	// get a failure if sudo prompts.
	cmd := ssh.Command("ubuntu@"+host, []string{"sudo", "true"}, ssh.NoPasswordAuthentication)
	if cmd.Run() == nil {
		return nil
	}

	// Failed to login as ubuntu (or passwordless sudo is not enabled).
	// Use specified login, and execute the initUbuntuScript below.
	if login != "" {
		host = login + "@" + host
	}
	logger.Infof("authorized_keys: %s", authorizedKeys)
	script := fmt.Sprintf(initUbuntuScript, utils.ShQuote(authorizedKeys))
	cmd = ssh.Command(host, []string{"sudo", "bash", "-c", script}, ssh.AllocateTTY)
	var stderr bytes.Buffer
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout // for sudo prompt
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() != 0 {
			err = fmt.Errorf("%v (%v)", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

const initUbuntuScript = `
set -e
install -m 0600 /dev/null /etc/sudoers.d/90-juju-ubuntu
echo 'ubuntu ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/90-juju-ubuntu
su ubuntu -c 'install -D -m 0600 /dev/null ~/.ssh/authorized_keys'
export authorized_keys=%s
if [ ! -z "$authorized_keys" ]; then
    su ubuntu -c 'printf "%%s\n" "$authorized_keys" >> ~/.ssh/authorized_keys'
fi`
