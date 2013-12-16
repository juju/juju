// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io"
	"juju-core/ssh-options/utils/ssh"
	"os"
	"os/signal"
	"path"
	"reflect"
	"strings"
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
	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.provider.common")

// Bootstrap is a common implementation of the Bootstrap method defined on
// environs.Environ; we strongly recommend that this implementation be used
// when writing a new provider.
func Bootstrap(env environs.Environ, cons constraints.Value) (err error) {
	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to make sure that only one succeeds.

	// TODO(axw) 2013-11-22 #1237736
	// Modify environs/Environ Bootstrap method signature
	// to take a new context structure, which contains
	// Std{in,out,err}, and interrupt signal handling.
	ctx := BootstrapContext{Stderr: os.Stderr}

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

	fmt.Fprintln(ctx.Stderr, "Launching instance")
	inst, hw, err := env.StartInstance(cons, selectedTools, machineConfig)
	if err != nil {
		return fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	fmt.Fprintf(ctx.Stderr, " - %s\n", inst.Id())

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
	inst = &refreshingInstance{Instance: inst, env: env}
	return FinishBootstrap(&ctx, inst, machineConfig)
}

// handelBootstrapError cleans up after a failed bootstrap.
func handleBootstrapError(err error, ctx *BootstrapContext, inst instance.Instance, env environs.Environ) {
	if err == nil {
		return
	}
	logger.Errorf("bootstrap failed: %v", err)
	ctx.SetInterruptHandler(func() {
		fmt.Fprintln(ctx.Stderr, "Cleaning up failed bootstrap")
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

// refreshingInstance is one that refreshes the
// underlying Instance before each call to Addresses.
type refreshingInstance struct {
	instance.Instance
	env     environs.Environ
	refresh bool
}

func (i *refreshingInstance) Addresses() ([]instance.Address, error) {
	if i.refresh {
		instances, err := i.env.Instances([]instance.Id{i.Instance.Id()})
		if err != nil {
			return nil, err
		}
		i.Instance = instances[0]
	}
	i.refresh = true
	return i.Instance.Addresses()
}

// FinishBootstrap completes the bootstrap process by connecting
// to the instance via SSH and carrying out the cloud-config.
//
// Note: FinishBootstrap is exposed so it can be replaced for testing.
var FinishBootstrap = func(ctx *BootstrapContext, inst instance.Instance, machineConfig *cloudinit.MachineConfig) error {
	var t tomb.Tomb
	ctx.SetInterruptHandler(func() { t.Killf("interrupted") })
	// Each attempt to connect to an address must verify the machine is the
	// bootstrap machine by checking its nonce file exists and contains the
	// nonce in the MachineConfig. This also blocks sshinit from proceeding
	// until cloud-init has completed, which is necessary to ensure apt
	// invocations don't trample each other.
	nonceFile := utils.ShQuote(path.Join(machineConfig.DataDir, cloudinit.NonceFile))
	checkNonceCommand := fmt.Sprintf(`
	noncefile=%s
	if [ ! -e "$noncefile" ]; then
		echo "$noncefile does not exist" >&2
		exit 1
	fi
	content=$(cat $noncefile)
	if [ "$content" != %s ]; then
		echo "$noncefile contents do not match machine nonce" >&2
		exit 1
	fi
	`, nonceFile, utils.ShQuote(machineConfig.MachineNonce))
	// TODO: jam 2013-12-04 bug #1257649
	// It would be nice if users had some controll over their bootstrap
	// timeout, since it is unlikely to be a perfect match for all clouds.
	addr, err := waitSSH(ctx, checkNonceCommand, inst, &t, DefaultBootstrapSSHTimeout())
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
	return sshinit.Configure("ubuntu@"+addr, cloudcfg)
}

// SSHTimeoutOpts lists the amount of time we will wait for various parts of
// the SSH connection to complete. This is similar to DialOpts, see
// http://pad.lv/1258889 about possibly deduplicating them.
type SSHTimeoutOpts struct {
	// Timeout is the amount of time to wait contacting
	// a state server.
	Timeout time.Duration

	// ConnectDelay is the amount of time between attempts to connect to an address.
	ConnectDelay time.Duration

	// AddressesDelay is the amount of time between refreshing the addresses.
	AddressesDelay time.Duration
}

// DefaultBootstrapSSHTimeout is the time we'll wait for SSH to come up on the bootstrap node
func DefaultBootstrapSSHTimeout() SSHTimeoutOpts {
	return SSHTimeoutOpts{
		Timeout: 10 * time.Minute,

		ConnectDelay: 5 * time.Second,

		// Not too frequent, as we refresh addresses from the provider each time.
		AddressesDelay: 10 * time.Second,
	}
}

type addresser interface {
	// Addresses returns the DNS name for the instance.
	// If the name is not yet allocated, it will return
	// an ErrNoAddresses error.
	Addresses() ([]instance.Address, error)
}

// connectSSH is called to connect to the specified host and
// execute the "checkHostScript" bash script on it.
var connectSSH = func(host, checkHostScript string) error {
	cmd := ssh.Command("ubuntu@"+host, []string{"/bin/bash", "-c", utils.ShQuote(checkHostScript)})
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) > 0 {
		err = fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	return err
}

// waitSSH waits for the instance to be assigned a routable
// address, then waits until we can connect to it via SSH.
//
// waitSSH attempts on all addresses returned by the instance
// in parallel; the first succeeding one wins. We ensure that
// private addresses are for the correct machine by checking
// the presence of a file on the machine that contains the
// machine's nonce. The "checkHostScript" is a bash script
// that performs this file check.
func waitSSH(ctx *BootstrapContext, checkHostScript string, inst addresser, t *tomb.Tomb, timeout SSHTimeoutOpts) (addr string, err error) {
	defer t.Done()
	globalTimeout := time.After(timeout.Timeout)
	pollAddresses := time.NewTimer(0)

	type checkHostResult struct {
		addr instance.Address
		err  error
	}
	checkHost := func(addr instance.Address, ch chan checkHostResult) {
		err := connectSSH(addr.Value, checkHostScript)
		if err != nil {
			time.Sleep(timeout.ConnectDelay)
		}
		ch <- checkHostResult{addr, err}
	}

	// store the last error so we can report it on global timeout
	var lastErr error
	// set of addresses most recently returned by the provider.
	var addresses []instance.Address
	// map of dns-names to result channels actively being tested
	active := make(map[instance.Address]chan checkHostResult)

	// Constants for reflect.SelectCases below.
	const (
		casePollAddresses = iota
		caseGlobalTimeout
		caseTombDying
	)

	fmt.Fprintln(ctx.Stderr, "Waiting for address")
	for {
		cases := []reflect.SelectCase{
			casePollAddresses: {Dir: reflect.SelectRecv, Chan: reflect.ValueOf(pollAddresses.C)},
			caseGlobalTimeout: {Dir: reflect.SelectRecv, Chan: reflect.ValueOf(globalTimeout)},
			caseTombDying:     {Dir: reflect.SelectRecv, Chan: reflect.ValueOf(t.Dying())},
		}
		for addr, checkHostResultChan := range active {
			if checkHostResultChan == nil {
				tcpaddr := addr.Value + ":22"
				fmt.Fprintf(ctx.Stderr, "Attempting to connect to %s\n", tcpaddr)
				checkHostResultChan = make(chan checkHostResult, 1)
				active[addr] = checkHostResultChan
				go checkHost(addr, checkHostResultChan)
			}
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(checkHostResultChan),
			})
		}
		i, v, _ := reflect.Select(cases)
		switch i {
		case casePollAddresses:
			pollAddresses.Reset(timeout.AddressesDelay)
			newAddresses, err := inst.Addresses()
			if err != nil {
				return "", t.Killf("getting addresses: %v", err)
			} else {
				addresses = newAddresses
				for _, addr := range addresses {
					if _, ok := active[addr]; !ok {
						active[addr] = nil
					}
				}
			}
		case caseGlobalTimeout:
			format := "waited for %v "
			args := []interface{}{timeout.Timeout}
			if len(addresses) == 0 {
				format += "without getting any addresses"
			} else {
				format += "without being able to connect"
			}
			if lastErr != nil {
				format += ": %v"
				args = append(args, lastErr)
			}
			return "", t.Killf(format, args...)
		case caseTombDying:
			return "", t.Err()
		default:
			result := v.Interface().(checkHostResult)
			if result.err == nil {
				return result.addr.Value, nil
			}
			logger.Debugf("connection failed: %v", result.err)
			lastErr = result.err
			var found bool
			for _, addr := range addresses {
				if result.addr == addr {
					active[addr] = nil // retry
					found = true
					break
				}
			}
			if !found {
				delete(active, result.addr)
			}
		}
	}
}

// TODO(axw) move this to environs; see
// comment near the top of common.Bootstrap.
type BootstrapContext struct {
	once        sync.Once
	handlerchan chan func()

	Stderr io.Writer
}

func (ctx *BootstrapContext) SetInterruptHandler(f func()) {
	ctx.once.Do(ctx.initHandler)
	ctx.handlerchan <- f
}

func (ctx *BootstrapContext) initHandler() {
	ctx.handlerchan = make(chan func())
	go ctx.handleInterrupt()
}

func (ctx *BootstrapContext) handleInterrupt() {
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
