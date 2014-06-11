// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Altoros/gosigma"
	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudinit/sshinit"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/network"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/worker/localstorage"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	coreCloudinit "github.com/juju/juju/cloudinit"
	"github.com/juju/juju/instance"
)

//
// Imlementation of InstanceBroker: methods for starting and stopping instances.
//

// StartInstance asks for a new instance to be created, associated with
// the provided config in machineConfig. The given config describes the juju
// state for the new instance to connect to. The config MachineNonce, which must be
// unique within an environment, is used by juju to protect against the
// consequences of multiple instances being started with the same machine id.
func (env environ) StartInstance(args environs.StartInstanceParams) (
	instance.Instance, *instance.HardwareCharacteristics, []network.Info, error) {
	logger.Infof("sigmaEnviron.StartInstance...")

	if args.MachineConfig == nil {
		return nil, nil, nil, fmt.Errorf("machine configuration is nil")
	}

	if args.MachineConfig.HasNetworks() {
		return nil, nil, nil, fmt.Errorf("starting instances with networks is not supported yet")
	}

	if len(args.Tools) == 0 {
		return nil, nil, nil, fmt.Errorf("tools not found")
	}

	args.MachineConfig.Tools = args.Tools[0]
	if err := environs.FinishMachineConfig(args.MachineConfig, env.Config(), args.Constraints); err != nil {
		return nil, nil, nil, err
	}

	client := env.client
	server, rootdrive, err := client.newInstance(args)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed start instance: %v", err)
	}

	inst := sigmaInstance{server: server}
	addr := inst.findIPv4()
	if addr == "" {
		return nil, nil, nil, fmt.Errorf("failed obtain instance IP address")
	}

	// prepare new instance: wait up and running, populate nonce file, etc
	if err := env.prepareInstance(addr, args.MachineConfig); err != nil {
		return nil, nil, nil, fmt.Errorf("failed prepare instanse: %v", err)
	}

	// provide additional agent config for localstorage, if any
	if env.storage.tmp && args.MachineConfig.Bootstrap {
		if err := env.prepareStorage(addr, args.MachineConfig); err != nil {
			return nil, nil, nil, fmt.Errorf("failed prepare storage: %v", err)
		}
	}

	// prepare hardware characteristics
	hwch := inst.hardware()

	// populate root drive hardware characteristics
	switch rootdrive.Arch() {
	case "64":
		var a = arch.AMD64
		hwch.Arch = &a
	}

	diskSpace := rootdrive.Size() / gosigma.Megabyte
	if diskSpace > 0 {
		hwch.RootDisk = &diskSpace
	}

	logger.Tracef("hardware: %v", hwch)

	return inst, hwch, nil, nil
}

// AllInstances returns all instances currently known to the broker.
func (env environ) AllInstances() ([]instance.Instance, error) {
	// Please note that this must *not* return instances that have not been
	// allocated as part of this environment -- if it does, juju will see they
	// are not tracked in state, assume they're stale/rogue, and shut them down.

	logger.Tracef("environ.AllInstances...")

	servers, err := env.client.instances()
	if err != nil {
		logger.Tracef("environ.AllInstances failed: %v", err)
		return nil, err
	}

	instances := make([]instance.Instance, 0, len(servers))
	for _, server := range servers {
		instance := sigmaInstance{server: server}
		instances = append(instances, instance)
	}

	if logger.LogLevel() <= loggo.TRACE {
		logger.Tracef("All instances, len = %d:", len(instances))
		for _, instance := range instances {
			logger.Tracef("... id: %q, status: %q", instance.Id(), instance.Status())
		}
	}

	return instances, nil
}

// Instances returns a slice of instances corresponding to the
// given instance ids.  If no instances were found, but there
// was no other error, it will return ErrNoInstances.  If
// some but not all the instances were found, the returned slice
// will have some nil slots, and an ErrPartialInstances error
// will be returned.
func (env environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	logger.Tracef("environ.Instances %#v", ids)
	// Please note that this must *not* return instances that have not been
	// allocated as part of this environment -- if it does, juju will see they
	// are not tracked in state, assume they're stale/rogue, and shut them down.
	// This advice applies even if an instance id passed in corresponds to a
	// real instance that's not part of the environment -- the Environ should
	// treat that no differently to a request for one that does not exist.

	m, err := env.client.instanceMap()
	if err != nil {
		logger.Tracef("environ.Instances failed: %v", err)
		return nil, err
	}

	var found int
	r := make([]instance.Instance, len(ids))
	for i, id := range ids {
		if s, ok := m[string(id)]; ok {
			r[i] = sigmaInstance{server: s}
			found++
		}
	}

	if found == 0 {
		err = environs.ErrNoInstances
	} else if found != len(ids) {
		err = environs.ErrPartialInstances
	}

	return r, err
}

// StopInstances shuts down the given instances.
func (env environ) StopInstances(instances ...instance.Id) error {
	logger.Infof("stop instances %+v", instances)

	var err error

	for _, instance := range instances {
		if e := env.client.stopInstance(instance); e != nil {
			err = e
		}
	}

	return err
}

func (env environ) prepareInstance(addr string, mcfg *cloudinit.MachineConfig) error {
	host := "ubuntu@" + addr
	logger.Debugf("Running prepare script on %s", host)

	cmds := []string{"#!/bin/bash", "set -e"}
	cmds = append(cmds, fmt.Sprintf("mkdir -p '%s'", mcfg.DataDir))

	nonceFile := path.Join(mcfg.DataDir, cloudinit.NonceFile)
	nonceContent := mcfg.MachineNonce
	cmds = append(cmds, fmt.Sprintf("echo '%s' > %s", nonceContent, nonceFile))

	script := strings.Join(cmds, "\n")

	if !mcfg.Bootstrap {
		cloudcfg := coreCloudinit.New()
		if err := cloudinit.ConfigureJuju(mcfg, cloudcfg); err != nil {
			return err
		}
		configScript, err := sshinit.ConfigureScript(cloudcfg)
		if err != nil {
			return err
		}

		script += "\n\n"
		script += configScript
	}

	logger.Tracef("Script:\n%s", script)

	keyfile := path.Join(mcfg.DataDir, agent.SystemIdentity)
	if _, err := os.Stat(keyfile); err != nil {
		keyfile = ""
	}
	logger.Tracef("System identity file: %q", keyfile)

	var retryAttempts = utils.AttemptStrategy{
		Total: 120 * time.Second,
		Delay: 300 * time.Millisecond,
	}

	var err error
	for attempt := retryAttempts.Start(); attempt.Next(); {
		var options ssh.Options
		if keyfile != "" {
			options.SetIdentities(keyfile)
		}

		cmd := ssh.Command(host, []string{"sudo", "/bin/bash"}, &options)
		cmd.Stdin = strings.NewReader(script)

		var logw = loggerWriter{logger.LogLevel()}
		cmd.Stderr = &logw
		cmd.Stdout = &logw

		if err = cmd.Run(); err == nil {
			break
		}
	}

	if err != nil {
		return fmt.Errorf("failed running prepare script: %v", err)
	}

<<<<<<< HEAD
<<<<<<< HEAD
	logger.Tracef("Bootstrap script done for machine '%s', '%s'", mcfg.MachineId, addr)

=======
>>>>>>> 071fbea... merged implementation of CloudSigma provider from launchpad
=======
>>>>>>> 071fbea... merged implementation of CloudSigma provider from launchpad
	return nil
}

func (env environ) prepareStorage(addr string, mcfg *cloudinit.MachineConfig) error {
	storagePort := env.ecfg.storagePort()
	storageDir := mcfg.DataDir + "/" + storageSubdir

	logger.Debugf("Moving local temporary storage to %s:%d (%s)...", addr, storagePort, storageDir)
	if err := env.storage.MoveToSSH("ubuntu", addr); err != nil {
		return err
	}

	if strings.Contains(mcfg.Tools.URL, "%s") {
		mcfg.Tools.URL = fmt.Sprintf(mcfg.Tools.URL, "file://"+storageDir)
		logger.Tracef("Tools URL patched to %q", mcfg.Tools.URL)
	}

	// prepare configuration for local storage at bootstrap host
	storageConfig := storageConfig{
		ecfg:        env.ecfg,
		storageDir:  storageDir,
		storageAddr: addr,
		storagePort: storagePort,
	}

	agentEnv, err := localstorage.StoreConfig(&storageConfig)
	if err != nil {
		return err
	}

	for k, v := range agentEnv {
		mcfg.AgentEnvironment[k] = v
	}

	return nil
}

// AllocateAddress requests a new address to be allocated for the
// given instance on the given network.
func (env environ) AllocateAddress(instID instance.Id, netID network.Id) (network.Address, error) {
	return network.Address{}, errors.NotSupportedf("AllocateAddress")
}
<<<<<<< HEAD
<<<<<<< HEAD

// ListNetworks returns basic information about all networks known
// by the provider for the environment. They may be unknown to juju
// yet (i.e. when called initially or when a new network was created).
func (env environ) ListNetworks() ([]network.BasicInfo, error) {
	return nil, errors.NotImplementedf("ListNetworks")
}
=======
>>>>>>> 071fbea... merged implementation of CloudSigma provider from launchpad
=======
>>>>>>> 071fbea... merged implementation of CloudSigma provider from launchpad
