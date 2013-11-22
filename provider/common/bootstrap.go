// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/cloudinit/sshinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	coretools "launchpad.net/juju-core/tools"
)

var logger = loggo.GetLogger("juju.provider.common")

// Bootstrap is a common implementation of the Bootstrap method defined on
// environs.Environ; we strongly recommend that this implementation be used
// when writing a new provider.
func Bootstrap(env environs.Environ, cons constraints.Value) (err error) {
	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to make sure that only one succeeds.

	// TODO(axw) 2013-11-22 #XXX
	// Modify environs/Environ Bootstrap method signature
	// to take a new context structure, which contains
	// Std{in,out,err}, and interrupt signal handling.
	ctx := bootstrapContext{Stderr: os.Stderr}

	var inst instance.Instance
	defer func() { handleBootstrapError(err, &ctx, inst, env) }()

	// Create an empty bootstrap state file so we can get its URL.
	// It will be updated with the instance id and hardware characteristics
	// after the bootstrap instance is started.
	stateFileURL, err := bootstrap.CreateStateFile(env.Storage())
	if err != nil {
		return err
	}
	machineConfig := environs.NewBootstrapMachineConfig(stateFileURL)

	selectedTools, err := EnsureBootstrapTools(env, env.Config().DefaultSeries(), cons.Arch)
	if err != nil {
		return err
	}

	var hw *instance.HardwareCharacteristics
	inst, hw, err = env.StartInstance(cons, selectedTools, machineConfig)
	if err != nil {
		return fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	var characteristics []instance.HardwareCharacteristics
	if hw != nil {
		characteristics = []instance.HardwareCharacteristics{*hw}
	}
	err = bootstrap.SaveState(
		env.Storage(),
		&bootstrap.BootstrapState{
			StateInstances:  []instance.Id{inst.Id()},
			Characteristics: characteristics,
		})
	if err != nil {
		return fmt.Errorf("cannot save state: %v", err)
	}
	return finishBootstrap(&ctx, inst, machineConfig)
}

// handelBootstrapError cleans up after a failed bootstrap.
func handleBootstrapError(err error, ctx *bootstrapContext, inst instance.Instance, env environs.Environ) {
	if err == nil {
		return
	}
	ctx.SetInterruptHandler(func() {
		fmt.Fprintln(ctx.Stderr, "cleaning up failed bootstrap")
	})
	if inst != nil {
		fmt.Fprintln(ctx.Stderr, "Stopping instance...")
		if stoperr := env.StopInstances([]instance.Instance{inst}); stoperr != nil {
			logger.Errorf("cannot stop failed bootstrap instance %q: %v", inst.Id(), stoperr)
		} else {
			// set to nil so we know we can safely delete the state file
			inst = nil
		}
	}
	// We only delete the bootstrap state file if either we didn't
	// start an instance, or we managed to cleanly stop it.
	if inst == nil {
		if rmerr := bootstrap.DeleteStateFile(env.Storage()); rmerr != nil {
			logger.Errorf("cannot delete bootstrap state file: %v", rmerr)
		}
	}
	ctx.SetInterruptHandler(nil)
}

// TestingDisableFinishBootstrap disables finishBootstrap so that tests
// do not attempt to SSH to non-existent machines. The result is a function
// that restores finishBootstrap.
func TestingDisableFinishBootstrap() func() {
	return testingPatchFinishBootstrap(func(*bootstrapContext, instance.Instance, *cloudinit.MachineConfig) error {
		return nil
	})
}

// testingDisableFinishBootstrap replaces the default finishBootstrap with
// a user-provided function. The result is a function that restores
// finishBootstrap.
func testingPatchFinishBootstrap(f func(*bootstrapContext, instance.Instance, *cloudinit.MachineConfig) error) func() {
	orig := finishBootstrap
	finishBootstrap = f
	return func() { finishBootstrap = orig }
}

// finishBootstrap completes the bootstrap process by connecting
// to the instance via SSH and carrying out the cloud-config.
var finishBootstrap = func(ctx *bootstrapContext, inst instance.Instance, machineConfig *cloudinit.MachineConfig) error {
	var t tomb.Tomb
	ctx.SetInterruptHandler(func() { t.Killf("interrupted") })
	dnsName, err := waitSSH(ctx, inst, &t)
	if err != nil {
		return err
	}
	// Bootstrap is synchronous, and will spawn a subprocess
	// to complete the procedure. If the user hits Ctrl-C,
	// SIGINT is sent to the foreground process attached to
	// the terminal, which will be the ssh subprocess at that
	// point.
	ctx.SetInterruptHandler(func() {})
	cloudcfg := coreCloudinit.New()
	if err := cloudinit.ConfigureJuju(machineConfig, cloudcfg); err != nil {
		return err
	}
	return sshinit.Configure("ubuntu@"+dnsName, cloudcfg)
}

// waitSSH waits for the instance to be assigned a DNS
// entry, then waits until we can connect to it via SSH.
func waitSSH(ctx *bootstrapContext, inst instance.Instance, t *tomb.Tomb) (dnsName string, err error) {
	defer t.Done()

	// Wait for a DNS name.
	fmt.Fprintln(ctx.Stderr, "Waiting for DNS name...")
	for {
		dnsName, err = inst.DNSName()
		if err == nil {
			break
		} else if err != instance.ErrNoDNSName {
			return "", t.Killf("getting DNS name: %v", err)
		}
		select {
		case <-time.After(1 * time.Second):
		case <-t.Dying():
			return "", t.Err()
		}
	}
	fmt.Fprintf(ctx.Stderr, " - %v\n", dnsName)

	// Wait until we can open a connection to port 22.
	for {
		fmt.Fprintf(ctx.Stderr, "Attempting to connect to %s:22...\n", dnsName)
		conn, err := net.DialTimeout("tcp", dnsName+":22", 5*time.Second)
		if err == nil {
			conn.Close()
			return dnsName, nil
		} else {
			logger.Debugf("connection failed: %v", err)
		}
		select {
		case <-time.After(5 * time.Second):
		case <-t.Dying():
			return "", t.Err()
		}
	}
}

type bootstrapContext struct {
	once        sync.Once
	handlerchan chan func()

	Stderr *os.File
}

func (ctx *bootstrapContext) SetInterruptHandler(f func()) {
	ctx.once.Do(ctx.initHandler)
	ctx.handlerchan <- f
}

func (ctx *bootstrapContext) initHandler() {
	ctx.handlerchan = make(chan func())
	go ctx.handleInterrupt()
}

func (ctx *bootstrapContext) handleInterrupt() {
	signalchan := make(chan os.Signal, 1)
	var s chan os.Signal
	var handler func()
	for {
		select {
		case handler = <-ctx.handlerchan:
			if handler == nil {
				if s != nil {
					signal.Stop(signalchan)
					s = nil
				}
			} else {
				if s == nil {
					s = signalchan
					signal.Notify(signalchan, os.Interrupt)
				}
			}
		case <-s:
			handler()
		}
	}
}

// EnsureBootstrapTools finds tools, syncing with an external tools source as
// necessary; it then selects the newest tools to bootstrap with, and sets
// agent-version.
func EnsureBootstrapTools(env environs.Environ, series string, arch *string) (coretools.List, error) {
	possibleTools, err := bootstrap.EnsureToolsAvailability(env, series, arch)
	if err != nil {
		return nil, err
	}
	return bootstrap.SetBootstrapTools(env, possibleTools)
}

// EnsureNotBootstrapped returns null if the environment is not bootstrapped,
// and an error if it is or if the function was not able to tell.
func EnsureNotBootstrapped(env environs.Environ) error {
	_, err := bootstrap.LoadState(env.Storage())
	// If there is no error loading the bootstrap state, then we are
	// bootstrapped.
	if err == nil {
		return fmt.Errorf("environment is already bootstrapped")
	}
	if err == environs.ErrNotBootstrapped {
		return nil
	}
	return err
}
