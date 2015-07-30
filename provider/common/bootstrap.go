// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/parallel"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/sshinit"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/utils/ssh"
)

var logger = loggo.GetLogger("juju.provider.common")

// Bootstrap is a common implementation of the Bootstrap method defined on
// environs.Environ; we strongly recommend that this implementation be used
// when writing a new provider.
func Bootstrap(ctx environs.BootstrapContext, env environs.Environ, args environs.BootstrapParams,
) (arch, series string, _ environs.BootstrapFinalizer, err error) {
	if result, series, finalizer, err := BootstrapInstance(ctx, env, args); err == nil {
		return *result.Hardware.Arch, series, finalizer, nil
	} else {
		return "", "", nil, err
	}
}

// BootstrapInstance creates a new instance with the series and architecture
// of its choice, constrained to those of the available tools, and
// returns the instance result, series, and a function that
// must be called to finalize the bootstrap process by transferring
// the tools and installing the initial Juju state server.
// This method is called by Bootstrap above, which implements environs.Bootstrap, but
// is also exported so that providers can manipulate the started instance.
func BootstrapInstance(ctx environs.BootstrapContext, env environs.Environ, args environs.BootstrapParams,
) (_ *environs.StartInstanceResult, series string, _ environs.BootstrapFinalizer, err error) {
	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to make sure that only one succeeds.

	// First thing, ensure we have tools otherwise there's no point.
	series = config.PreferredSeries(env.Config())
	availableTools, err := args.AvailableTools.Match(coretools.Filter{Series: series})
	if err != nil {
		return nil, "", nil, err
	}

	// Get the bootstrap SSH client. Do this early, so we know
	// not to bother with any of the below if we can't finish the job.
	client := ssh.DefaultClient
	if client == nil {
		// This should never happen: if we don't have OpenSSH, then
		// go.crypto/ssh should be used with an auto-generated key.
		return nil, "", nil, fmt.Errorf("no SSH client available")
	}

	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(args.Constraints, series)
	if err != nil {
		return nil, "", nil, err
	}
	instanceConfig.EnableOSRefreshUpdate = env.Config().EnableOSRefreshUpdate()
	instanceConfig.EnableOSUpgrade = env.Config().EnableOSUpgrade()
	instanceConfig.Tags = instancecfg.InstanceTags(env.Config(), instanceConfig.Jobs)
	maybeSetBridge := func(icfg *instancecfg.InstanceConfig) {
		// If we need to override the default bridge name, do it now. When
		// args.ContainerBridgeName is empty, the default names for LXC
		// (lxcbr0) and KVM (virbr0) will be used.
		if args.ContainerBridgeName != "" {
			logger.Debugf("using %q as network bridge for all container types", args.ContainerBridgeName)
			if icfg.AgentEnvironment == nil {
				icfg.AgentEnvironment = make(map[string]string)
			}
			icfg.AgentEnvironment[agent.LxcBridge] = args.ContainerBridgeName
		}
	}
	maybeSetBridge(instanceConfig)

	fmt.Fprintln(ctx.GetStderr(), "Launching instance")
	result, err := env.StartInstance(environs.StartInstanceParams{
		Constraints:    args.Constraints,
		Tools:          availableTools,
		InstanceConfig: instanceConfig,
		Placement:      args.Placement,
	})
	if err != nil {
		return nil, "", nil, errors.Annotate(err, "cannot start bootstrap instance")
	}
	fmt.Fprintf(ctx.GetStderr(), " - %s\n", result.Instance.Id())

	finalize := func(ctx environs.BootstrapContext, icfg *instancecfg.InstanceConfig) error {
		icfg.InstanceId = result.Instance.Id()
		icfg.HardwareCharacteristics = result.Hardware
		if err := instancecfg.FinishInstanceConfig(icfg, env.Config()); err != nil {
			return err
		}
		maybeSetBridge(icfg)
		return FinishBootstrap(ctx, client, result.Instance, icfg)
	}
	return result, series, finalize, nil
}

// FinishBootstrap completes the bootstrap process by connecting
// to the instance via SSH and carrying out the cloud-config.
//
// Note: FinishBootstrap is exposed so it can be replaced for testing.
var FinishBootstrap = func(ctx environs.BootstrapContext, client ssh.Client, inst instance.Instance, instanceConfig *instancecfg.InstanceConfig) error {
	interrupted := make(chan os.Signal, 1)
	ctx.InterruptNotify(interrupted)
	defer ctx.StopInterruptNotify(interrupted)
	// Each attempt to connect to an address must verify the machine is the
	// bootstrap machine by checking its nonce file exists and contains the
	// nonce in the InstanceConfig. This also blocks sshinit from proceeding
	// until cloud-init has completed, which is necessary to ensure apt
	// invocations don't trample each other.
	nonceFile := utils.ShQuote(path.Join(instanceConfig.DataDir, cloudconfig.NonceFile))
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
	`, nonceFile, utils.ShQuote(instanceConfig.MachineNonce))
	addr, err := waitSSH(
		ctx,
		interrupted,
		client,
		checkNonceCommand,
		inst,
		instanceConfig.Config.BootstrapSSHOpts(),
	)
	if err != nil {
		return err
	}
	return ConfigureMachine(ctx, client, addr, instanceConfig)
}

func ConfigureMachine(ctx environs.BootstrapContext, client ssh.Client, host string, instanceConfig *instancecfg.InstanceConfig) error {
	// Bootstrap is synchronous, and will spawn a subprocess
	// to complete the procedure. If the user hits Ctrl-C,
	// SIGINT is sent to the foreground process attached to
	// the terminal, which will be the ssh subprocess at this
	// point. For that reason, we do not call StopInterruptNotify
	// until this function completes.
	cloudcfg, err := cloudinit.New(instanceConfig.Series)
	if err != nil {
		return errors.Trace(err)
	}

	// Set packaging update here
	cloudcfg.SetSystemUpdate(instanceConfig.EnableOSRefreshUpdate)
	cloudcfg.SetSystemUpgrade(instanceConfig.EnableOSUpgrade)

	udata, err := cloudconfig.NewUserdataConfig(instanceConfig, cloudcfg)
	if err != nil {
		return err
	}
	if err := udata.ConfigureJuju(); err != nil {
		return err
	}
	configScript, err := cloudcfg.RenderScript()
	if err != nil {
		return err
	}
	script := shell.DumpFileOnErrorScript(instanceConfig.CloudInitOutputLog) + configScript
	return sshinit.RunConfigureScript(script, sshinit.ConfigureParams{
		Host:           "ubuntu@" + host,
		Client:         client,
		Config:         cloudcfg,
		ProgressWriter: ctx.GetStderr(),
		Series:         instanceConfig.Series,
	})
}

type addresser interface {
	// Refresh refreshes the addresses for the instance.
	Refresh() error

	// Addresses returns the addresses for the instance.
	// To ensure that the results are up to date, call
	// Refresh first.
	Addresses() ([]network.Address, error)
}

type hostChecker struct {
	addr   network.Address
	client ssh.Client
	wg     *sync.WaitGroup

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
	defer hc.wg.Done()
	// The value of connectSSH is taken outside the goroutine that may outlive
	// hostChecker.loop, or we evoke the wrath of the race detector.
	connectSSH := connectSSH
	done := make(chan error, 1)
	var lastErr error
	for {
		address := hc.addr.Value
		go func() {
			done <- connectSSH(hc.client, address, hc.checkHostScript)
		}()
		select {
		case <-hc.closed:
			return hc, lastErr
		case <-dying:
			return hc, lastErr
		case lastErr = <-done:
			if lastErr == nil {
				return hc, nil
			} else {
				logger.Debugf("connection attempt for %s failed: %v", address, lastErr)
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
	wg     sync.WaitGroup

	// active is a map of adresses to channels for addresses actively
	// being tested. The goroutine testing the address will continue
	// to attempt connecting to the address until it succeeds, the Try
	// is killed, or the corresponding channel in this map is closed.
	active map[network.Address]chan struct{}

	// checkDelay is how long each hostChecker waits between attempts.
	checkDelay time.Duration

	// checkHostScript is the script to run on each host to check that
	// it is the host we expect.
	checkHostScript string
}

func (p *parallelHostChecker) UpdateAddresses(addrs []network.Address) {
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
			wg:              &p.wg,
		}
		p.wg.Add(1)
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
		active:          make(map[network.Address]chan struct{}),
		checkDelay:      timeout.RetryDelay,
		checkHostScript: checkHostScript,
	}
	defer checker.wg.Wait()
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
