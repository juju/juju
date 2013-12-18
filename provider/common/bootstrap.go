// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
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
	"launchpad.net/juju-core/utils/parallel"
	"launchpad.net/juju-core/utils/ssh"
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
	// Refresh refreshes the addresses for the instance.
	Refresh() error

	// Addresses returns the addresses for the instance.
	// To ensure that the results are up to date, call
	// Refresh first.
	Addresses() ([]instance.Address, error)
}

type hostChecker struct {
	addr instance.Address

	// checkDelay is the amount of time to wait between retries
	checkDelay time.Duration

	// checkHostScript is executed on the host via SSH.
	// hostChecker.loop will return once the script
	// runs without error.
	checkHostScript string

	// closeRetry is closed when the hostChecker
	// should stop retrying.
	closeRetry chan struct{}
}

// Close implements io.Closer, as required by parallel.Try.
func (*hostChecker) Close() error {
	return nil
}

func (hc *hostChecker) loop(closed <-chan struct{}) (io.Closer, error) {
	done := make(chan error, 1)
	var lastErr error
	for {
		go func() {
			done <- connectSSH(hc.addr.Value, hc.checkHostScript)
		}()
		select {
		case <-closed:
			if lastErr == nil {
				lastErr = parallel.ErrClosed
			}
			return nil, lastErr
		case <-hc.closeRetry:
			hc.closeRetry = nil
		case lastErr = <-done:
			if lastErr == nil {
				return hc, nil
			} else if hc.closeRetry == nil {
				if lastErr == nil {
					lastErr = fmt.Errorf("stop retrying")
				}
				return nil, lastErr
			}
		}
		time.Sleep(hc.checkDelay)
	}
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

	// map of adresses to channels for addresses actively being tested.
	// the goroutine testing the address will continue to attempt
	// connecting to the address until it succeeds, the Try is closed,
	// or the corresponding channel in this map is closed.
	active := make(map[instance.Address]chan struct{})

	// try each address in parallel
	try := parallel.NewTry(0, nil)
	defer try.Kill()

	fmt.Fprintln(ctx.Stderr, "Waiting for address")
	for {
		select {
		case <-pollAddresses.C:
			pollAddresses.Reset(timeout.AddressesDelay)
			if err := inst.Refresh(); err != nil {
				return "", t.Killf("refreshing addresses: %v", err)
			}
			addresses, err := inst.Addresses()
			if err != nil {
				return "", t.Killf("getting addresses: %v", err)
			}
			oldActive := active
			active = make(map[instance.Address]chan struct{})
			for _, addr := range addresses {
				if ch, ok := oldActive[addr]; ok {
					active[addr] = ch
				} else {
					fmt.Fprintf(ctx.Stderr, "Attempting to connect to %s:22\n", addr.Value)
					hc := &hostChecker{
						addr:            addr,
						checkDelay:      timeout.ConnectDelay,
						checkHostScript: checkHostScript,
						closeRetry:      make(chan struct{}),
					}
					active[addr] = hc.closeRetry
					try.Start(hc.loop)
				}
			}
			for addr, closeRetry := range oldActive {
				if _, ok := active[addr]; !ok {
					close(closeRetry)
				}
			}
		case <-globalTimeout:
			try.Kill()
			// Note that once killed, the Try does not
			// wait for the tasks to complete. If not
			// functions ever returned, before we timed
			// out, the error will be ErrStopped.
			lastErr := try.Wait()
			format := "waited for %v "
			args := []interface{}{timeout.Timeout}
			if len(active) == 0 {
				format += "without getting any addresses"
			} else {
				format += "without being able to connect"
			}
			if lastErr != nil && lastErr != parallel.ErrStopped {
				format += ": %v"
				args = append(args, lastErr)
			}
			return "", fmt.Errorf(format, args...)
		case <-t.Dying():
			return "", t.Err()
		case <-try.Dead():
			result, err := try.Result()
			if err != nil {
				return "", err
			}
			return result.(*hostChecker).addr.Value, nil
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
