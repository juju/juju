// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/juju/loggo"

	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/cloudinit/sshinit"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/parallel"
	"launchpad.net/juju-core/utils/shell"
	"launchpad.net/juju-core/utils/ssh"
)

var logger = loggo.GetLogger("juju.provider.common")

// Bootstrap is a common implementation of the Bootstrap method defined on
// environs.Environ; we strongly recommend that this implementation be used
// when writing a new provider.
func Bootstrap(ctx environs.BootstrapContext, env environs.Environ, args environs.BootstrapParams) (err error) {
	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to make sure that only one succeeds.

	var inst instance.Instance
	defer func() { handleBootstrapError(err, ctx, inst, env) }()

	// First thing, ensure we have tools otherwise there's no point.
	selectedTools, err := EnsureBootstrapTools(ctx, env, config.PreferredSeries(env.Config()), args.Constraints.Arch)
	if err != nil {
		return err
	}

	// Get the bootstrap SSH client. Do this early, so we know
	// not to bother with any of the below if we can't finish the job.
	client := ssh.DefaultClient
	if client == nil {
		// This should never happen: if we don't have OpenSSH, then
		// go.crypto/ssh should be used with an auto-generated key.
		return fmt.Errorf("no SSH client available")
	}

	privateKey, err := GenerateSystemSSHKey(env)
	if err != nil {
		return err
	}
	machineConfig := environs.NewBootstrapMachineConfig(privateKey)

	fmt.Fprintln(ctx.GetStderr(), "Launching instance")
	inst, hw, _, err := env.StartInstance(environs.StartInstanceParams{
		Constraints:   args.Constraints,
		Tools:         selectedTools,
		MachineConfig: machineConfig,
		Placement:     args.Placement,
	})
	if err != nil {
		return fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	fmt.Fprintf(ctx.GetStderr(), " - %s\n", inst.Id())
	machineConfig.InstanceId = inst.Id()
	machineConfig.HardwareCharacteristics = hw

	err = bootstrap.SaveState(
		env.Storage(),
		&bootstrap.BootstrapState{
			StateInstances: []instance.Id{inst.Id()},
		})
	if err != nil {
		return fmt.Errorf("cannot save state: %v", err)
	}
	return FinishBootstrap(ctx, client, inst, machineConfig)
}

// GenerateSystemSSHKey creates a new key for the system identity. The
// authorized_keys in the environment config is updated to include the public
// key for the generated key.
func GenerateSystemSSHKey(env environs.Environ) (privateKey string, err error) {
	logger.Debugf("generate a system ssh key")
	// Create a new system ssh key and add that to the authorized keys.
	privateKey, publicKey, err := ssh.GenerateKey(config.JujuSystemKey)
	if err != nil {
		return "", fmt.Errorf("failed to create system key: %v", err)
	}
	authorized_keys := config.ConcatAuthKeys(env.Config().AuthorizedKeys(), publicKey)
	newConfig, err := env.Config().Apply(map[string]interface{}{
		config.AuthKeysConfig: authorized_keys,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create new config: %v", err)
	}
	if err = env.SetConfig(newConfig); err != nil {
		return "", fmt.Errorf("failed to set new config: %v", err)
	}
	return privateKey, nil
}

// handleBootstrapError cleans up after a failed bootstrap.
func handleBootstrapError(err error, ctx environs.BootstrapContext, inst instance.Instance, env environs.Environ) {
	if err == nil {
		return
	}

	logger.Errorf("bootstrap failed: %v", err)
	ch := make(chan os.Signal, 1)
	ctx.InterruptNotify(ch)
	defer ctx.StopInterruptNotify(ch)
	defer close(ch)
	go func() {
		for _ = range ch {
			fmt.Fprintln(ctx.GetStderr(), "Cleaning up failed bootstrap")
		}
	}()

	if inst != nil {
		fmt.Fprintln(ctx.GetStderr(), "Stopping instance...")
		if stoperr := env.StopInstances(inst.Id()); stoperr != nil {
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
}

// FinishBootstrap completes the bootstrap process by connecting
// to the instance via SSH and carrying out the cloud-config.
//
// Note: FinishBootstrap is exposed so it can be replaced for testing.
var FinishBootstrap = func(ctx environs.BootstrapContext, client ssh.Client, inst instance.Instance, machineConfig *cloudinit.MachineConfig) error {
	interrupted := make(chan os.Signal, 1)
	ctx.InterruptNotify(interrupted)
	defer ctx.StopInterruptNotify(interrupted)
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
	addr, err := waitSSH(
		ctx,
		interrupted,
		client,
		checkNonceCommand,
		inst,
		machineConfig.Config.BootstrapSSHOpts(),
	)
	if err != nil {
		return err
	}
	// Bootstrap is synchronous, and will spawn a subprocess
	// to complete the procedure. If the user hits Ctrl-C,
	// SIGINT is sent to the foreground process attached to
	// the terminal, which will be the ssh subprocess at this
	// point. For that reason, we do not call StopInterruptNotify
	// until this function completes.
	cloudcfg := coreCloudinit.New()
	if err := cloudinit.ConfigureJuju(machineConfig, cloudcfg); err != nil {
		return err
	}
	configScript, err := sshinit.ConfigureScript(cloudcfg)
	if err != nil {
		return err
	}
	script := shell.DumpFileOnErrorScript(machineConfig.CloudInitOutputLog) + configScript
	return sshinit.RunConfigureScript(script, sshinit.ConfigureParams{
		Host:           "ubuntu@" + addr,
		Client:         client,
		Config:         cloudcfg,
		ProgressWriter: ctx.GetStderr(),
	})
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
	addr   instance.Address
	client ssh.Client

	// checkDelay is the amount of time to wait between retries.
	checkDelay time.Duration

	// checkHostScript is executed on the host via SSH.
	// hostChecker.loop will return once the script
	// runs without error.
	checkHostScript string

	// closed is closed to indicate that the host checker should
	// return, without waiting for the result of any ongoing
	// attempts.
	closed <-chan struct{}
}

// Close implements io.Closer, as required by parallel.Try.
func (*hostChecker) Close() error {
	return nil
}

func (hc *hostChecker) loop(dying <-chan struct{}) (io.Closer, error) {
	// The value of connectSSH is taken outside the goroutine that may outlive
	// hostChecker.loop, or we evoke the wrath of the race detector.
	connectSSH := connectSSH
	done := make(chan error, 1)
	var lastErr error
	for {
		go func() {
			done <- connectSSH(hc.client, hc.addr.Value, hc.checkHostScript)
		}()
		select {
		case <-hc.closed:
			return hc, lastErr
		case <-dying:
			return hc, lastErr
		case lastErr = <-done:
			if lastErr == nil {
				return hc, nil
			}
		}
		select {
		case <-hc.closed:
		case <-dying:
		case <-time.After(hc.checkDelay):
		}
	}
}

type parallelHostChecker struct {
	*parallel.Try
	client ssh.Client
	stderr io.Writer

	// active is a map of adresses to channels for addresses actively
	// being tested. The goroutine testing the address will continue
	// to attempt connecting to the address until it succeeds, the Try
	// is killed, or the corresponding channel in this map is closed.
	active map[instance.Address]chan struct{}

	// checkDelay is how long each hostChecker waits between attempts.
	checkDelay time.Duration

	// checkHostScript is the script to run on each host to check that
	// it is the host we expect.
	checkHostScript string
}

func (p *parallelHostChecker) UpdateAddresses(addrs []instance.Address) {
	for _, addr := range addrs {
		if _, ok := p.active[addr]; ok {
			continue
		}
		fmt.Fprintf(p.stderr, "Attempting to connect to %s:22\n", addr.Value)
		closed := make(chan struct{})
		hc := &hostChecker{
			addr:            addr,
			client:          p.client,
			checkDelay:      p.checkDelay,
			checkHostScript: p.checkHostScript,
			closed:          closed,
		}
		p.active[addr] = closed
		p.Start(hc.loop)
	}
}

// Close prevents additional functions from being added to
// the Try, and tells each active hostChecker to exit.
func (p *parallelHostChecker) Close() error {
	// We signal each checker to stop and wait for them
	// each to complete; this allows us to get the error,
	// as opposed to when using try.Kill which does not
	// wait for the functions to complete.
	p.Try.Close()
	for _, ch := range p.active {
		close(ch)
	}
	return nil
}

// connectSSH is called to connect to the specified host and
// execute the "checkHostScript" bash script on it.
var connectSSH = func(client ssh.Client, host, checkHostScript string) error {
	cmd := client.Command("ubuntu@"+host, []string{"/bin/bash"}, nil)
	cmd.Stdin = strings.NewReader(checkHostScript)
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
func waitSSH(ctx environs.BootstrapContext, interrupted <-chan os.Signal, client ssh.Client, checkHostScript string, inst addresser, timeout config.SSHTimeoutOpts) (addr string, err error) {
	globalTimeout := time.After(timeout.Timeout)
	pollAddresses := time.NewTimer(0)

	// checker checks each address in a loop, in parallel,
	// until one succeeds, the global timeout is reached,
	// or the tomb is killed.
	checker := parallelHostChecker{
		Try:             parallel.NewTry(0, nil),
		client:          client,
		stderr:          ctx.GetStderr(),
		active:          make(map[instance.Address]chan struct{}),
		checkDelay:      timeout.RetryDelay,
		checkHostScript: checkHostScript,
	}
	defer checker.Kill()

	fmt.Fprintln(ctx.GetStderr(), "Waiting for address")
	for {
		select {
		case <-pollAddresses.C:
			pollAddresses.Reset(timeout.AddressesDelay)
			if err := inst.Refresh(); err != nil {
				return "", fmt.Errorf("refreshing addresses: %v", err)
			}
			addresses, err := inst.Addresses()
			if err != nil {
				return "", fmt.Errorf("getting addresses: %v", err)
			}
			checker.UpdateAddresses(addresses)
		case <-globalTimeout:
			checker.Close()
			lastErr := checker.Wait()
			format := "waited for %v "
			args := []interface{}{timeout.Timeout}
			if len(checker.active) == 0 {
				format += "without getting any addresses"
			} else {
				format += "without being able to connect"
			}
			if lastErr != nil && lastErr != parallel.ErrStopped {
				format += ": %v"
				args = append(args, lastErr)
			}
			return "", fmt.Errorf(format, args...)
		case <-interrupted:
			return "", fmt.Errorf("interrupted")
		case <-checker.Dead():
			result, err := checker.Result()
			if err != nil {
				return "", err
			}
			return result.(*hostChecker).addr.Value, nil
		}
	}
}

// EnsureBootstrapTools finds tools, syncing with an external tools source as
// necessary; it then selects the newest tools to bootstrap with, and sets
// agent-version.
func EnsureBootstrapTools(ctx environs.BootstrapContext, env environs.Environ, series string, arch *string) (coretools.List, error) {
	possibleTools, err := bootstrap.EnsureToolsAvailability(ctx, env, series, arch)
	if err != nil {
		return nil, err
	}
	return bootstrap.SetBootstrapTools(env, possibleTools)
}
