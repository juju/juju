// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/juju/utils/v3/ssh"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/client"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/sshclient"
	"github.com/juju/juju/core/network"
	jujussh "github.com/juju/juju/network/ssh"
	"github.com/juju/juju/rpc/params"
)

var logger = loggo.GetLogger("juju.cmd.juju.ssh")

// sshMachine implements functionality shared by sshCommand, SCPCommand
// and DebugHooksCommand.
type sshMachine struct {
	leaderResolver
	modelName string

	proxy                  bool
	noHostKeyChecks        bool
	target                 string
	args                   []string
	sshClient              sshAPIClient
	statusClient           statusClient
	apiAddr                *url.URL
	knownHostsPath         string
	hostChecker            jujussh.ReachableChecker
	retryStrategy          retry.CallArgs
	publicKeyRetryStrategy retry.CallArgs
}

type statusClient interface {
	Status(args *client.StatusArgs) (*params.FullStatus, error)
}

type sshAPIClient interface {
	PublicAddress(target string) (string, error)
	PrivateAddress(target string) (string, error)
	AllAddresses(target string) ([]string, error)
	PublicKeys(target string) ([]string, error)
	Proxy() (bool, error)
	Close() error
}

type resolvedTarget struct {
	user   string
	entity string
	host   string

	// When the resolved target is a container which cannot be directly
	// reached by the controller (e.g. container has a fan address that the
	// controller cannot route traffic to; see LP1932547), via will be
	// populated with the details of the machine that hosts the container.
	//
	// This allows us to use the host machine as a jumpbox for connecting
	// to the target container.
	via *resolvedTarget
}

func (t *resolvedTarget) userHost() string {
	if t.user == "" {
		return t.host
	}
	return t.user + "@" + t.host
}

func (t *resolvedTarget) isAgent() bool {
	return targetIsAgent(t.entity)
}

// defaultSSHPort is the TCP port used for SSH connections.
const defaultSSHPort = 22

func (c *sshMachine) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.proxy, "proxy", false, "Proxy through the API server")
	f.BoolVar(&c.noHostKeyChecks, "no-host-key-checks", false, "Skip host key checking (INSECURE)")
}

// getTarget returns the target.
func (c *sshMachine) getTarget() string {
	return c.target
}

// setTarget sets the target.
func (c *sshMachine) setTarget(target string) {
	c.target = target
}

// getArgs returns the args.
func (c *sshMachine) getArgs() []string {
	return c.args
}

// setArgs sets the args.
func (c *sshMachine) setArgs(args []string) {
	c.args = args
}

// defaultReachableChecker returns a jujussh.ReachableChecker with a connection
// timeout of SSHTimeout/2 and an overall timout of SSHTimeout
func defaultReachableChecker() jujussh.ReachableChecker {
	return jujussh.NewReachableChecker(&net.Dialer{Timeout: SSHTimeout / 2}, SSHTimeout)
}

func (c *sshMachine) setHostChecker(checker jujussh.ReachableChecker) {
	if checker == nil {
		checker = defaultReachableChecker()
	}
	c.hostChecker = checker
}

func (c *sshMachine) setLeaderAPI(leaderAPI LeaderAPI) {
	c.leaderAPI = leaderAPI
}

func (c *sshMachine) setRetryStrategy(retryStrategy retry.CallArgs) {
	c.retryStrategy = retryStrategy
}

func (c *sshMachine) setPublicKeyRetryStrategy(retryStrategy retry.CallArgs) {
	c.publicKeyRetryStrategy = retryStrategy
}

// initRun initializes the API connection if required, and determines
// if SSH proxying is required. It must be called at the top of the
// command's Run method.
//
// The sshClient, apiAddr and proxy fields are initialized after this call.
func (c *sshMachine) initRun(mc ModelCommand) (err error) {
	if c.modelName, err = mc.ModelIdentifier(); err != nil {
		return errors.Trace(err)
	}

	if err = c.ensureAPIClient(mc); err != nil {
		return errors.Trace(err)
	}
	c.proxy, err = c.proxySSH()
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// cleanupRun removes the temporary SSH known_hosts file (if one was
// created) and closes the API connection. It must be called at the
// end of the command's Run (i.e. as a defer).
func (c *sshMachine) cleanupRun() {
	if c.knownHostsPath != "" {
		_ = os.Remove(c.knownHostsPath)
		c.knownHostsPath = ""
	}
	if c.sshClient != nil {
		_ = c.sshClient.Close()
		c.sshClient = nil
	}
	if c.leaderAPI != nil {
		_ = c.leaderAPI.Close()
		c.leaderAPI = nil
	}
}

// getSSHOptions configures SSH options based on command line
// arguments and the SSH targets specified.
func (c *sshMachine) getSSHOptions(enablePty bool, targets ...*resolvedTarget) (*ssh.Options, error) {
	var options ssh.Options

	if c.noHostKeyChecks {
		options.SetStrictHostKeyChecking(ssh.StrictHostChecksNo)
		options.SetKnownHostsFile(os.DevNull)
	} else {
		knownHostsPath, err := c.generateKnownHosts(targets)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// There might not be a custom known_hosts file if the SSH
		// targets are specified using arbitrary hostnames or
		// addresses. In this case, the user's personal known_hosts
		// file is used.

		if knownHostsPath != "" {
			// When a known_hosts file has been generated, enforce
			// strict host key checking.
			options.SetStrictHostKeyChecking(ssh.StrictHostChecksYes)
			options.SetKnownHostsFile(knownHostsPath)
		}
	}

	if enablePty {
		options.EnablePTY()
	}

	if c.proxy {
		if err := c.setProxyCommand(&options, targets); err != nil {
			return nil, err
		}
	}

	return &options, nil
}

func (c *sshMachine) ssh(ctx Context, enablePty bool, target *resolvedTarget) error {
	options, err := c.getSSHOptions(enablePty, target)
	if err != nil {
		return err
	}

	cmd := ssh.Command(target.userHost(), c.args, options)
	cmd.Stdin = ctx.GetStdin()
	cmd.Stdout = ctx.GetStdout()
	cmd.Stderr = ctx.GetStderr()
	return cmd.Run()
}

func (c *sshMachine) copy(_ Context) error {
	args, targets, err := c.expandSCPArgs(c.getArgs())
	if err != nil {
		return err
	}

	if c.proxy {
		for _, target := range targets {
			// If we are trying to connect to a container on a FAN address,
			// we need to route the traffic via the machine that hosts it.
			// This is required as the controller is unable to route fan
			// traffic across subnets.
			if err = c.maybePopulateTargetViaField(target, c.statusClient.Status); err != nil {
				return errors.Trace(err)
			}
		}
	}

	options, err := c.getSSHOptions(false, targets...)
	if err != nil {
		return err
	}
	return ssh.Copy(args, options)
}

// expandSCPArgs takes a list of arguments and looks for ones in the form of
// 0:some/path or application/0:some/path, and translates them into
// ubuntu@machine:some/path so they can be passed as arguments to scp, and pass
// the rest verbatim on to scp
func (c *sshMachine) expandSCPArgs(args []string) ([]string, []*resolvedTarget, error) {
	outArgs := make([]string, len(args))
	var targets []*resolvedTarget
	for i, arg := range args {
		v := strings.SplitN(arg, ":", 2)
		if strings.HasPrefix(arg, "-") || len(v) <= 1 {
			// Can't be an interesting target, so just pass it along
			outArgs[i] = arg
			continue
		}

		target, err := c.resolveTarget(v[0])
		if err != nil {
			return nil, nil, err
		}
		arg := net.JoinHostPort(target.host, v[1])
		if target.user != "" {
			arg = target.user + "@" + arg
		}
		outArgs[i] = arg

		targets = append(targets, target)
	}
	return outArgs, targets, nil
}

// generateKnownHosts takes the provided targets, retrieves the SSH
// public host keys for them and generates a temporary known_hosts
// file for them.
func (c *sshMachine) generateKnownHosts(targets []*resolvedTarget) (string, error) {
	knownHosts := newKnownHostsBuilder()
	agentCount := 0
	nonAgentCount := 0
	for _, target := range targets {
		if target.isAgent() {
			agentCount++
			keys, err := c.getKeysWithRetry(target.entity)
			if err != nil {
				return "", errors.Trace(err)
			}
			knownHosts.add(target.host, keys)
		} else {
			nonAgentCount++
		}
	}

	if agentCount > 0 && nonAgentCount > 0 {
		return "", errors.New("can't determine host keys for all targets: consider --no-host-key-checks")
	}

	if knownHosts.size() == 0 {
		// No public keys to write so exit early.
		return "", nil
	}

	f, err := os.CreateTemp("", "ssh_known_hosts")
	if err != nil {
		return "", errors.Annotate(err, "creating known hosts file")
	}
	defer func() { _ = f.Close() }()
	c.knownHostsPath = f.Name() // Record for later deletion
	if err := knownHosts.write(f); err != nil {
		return "", errors.Trace(err)
	}
	return c.knownHostsPath, nil
}

// proxySSH returns false if both c.proxy and the proxy-ssh model
// configuration are false -- otherwise it returns true.
func (c *sshMachine) proxySSH() (bool, error) {
	if c.proxy {
		// No need to check the API if user explicitly requested
		// proxying.
		return true, nil
	}
	proxy, err := c.sshClient.Proxy()
	if err != nil {
		return false, errors.Trace(err)
	}
	logger.Debugf("proxy-ssh is %v", proxy)
	return proxy, nil
}

// setProxyCommand sets the proxy command option.
func (c *sshMachine) setProxyCommand(options *ssh.Options, targets []*resolvedTarget) error {
	juju, err := getJujuExecutable()
	if err != nil {
		return errors.Errorf("failed to get juju executable path: %v", err)
	}

	// TODO(mjs) 2016-05-09 LP #1579592 - It would be good to check the
	// host key of the controller machine being used for proxying
	// here. This isn't too serious as all traffic passing through the
	// controller host is encrypted and the host key of the ultimate
	// target host is verified but it would still be better to perform
	// this extra level of checking.
	if len(targets) == 1 && targets[0].via != nil {
		// Use the controller as a jumpbox to ssh into the via target
		// and run nc to reach the ssh port of the target.
		//
		// NOTE(achilleasa) this only works when we have a single
		// target; with multiple targets we would need a different
		// proxy command for each one
		options.SetProxyCommand(
			juju, "ssh",
			"--model="+c.modelName,
			// We still need to proxy through the controller to
			// reach the via target as the target machine might not
			// have a public IP address.
			"--proxy=true",
			"--no-host-key-checks",
			"--pty=false",
			fmt.Sprintf("%s@%s", targets[0].via.user, targets[0].via.host),
			"-q",
			"nc %h %p",
		)
	} else {
		// Use the controller as a jumpbox and run nc to route traffic
		// to the ssh port of the target.
		options.SetProxyCommand(
			juju, "ssh",
			"--model="+c.modelName,
			"--proxy=false",
			"--no-host-key-checks",
			"--pty=false",
			"ubuntu@"+c.apiAddr.Hostname(),
			"-q",
			"nc %h %p",
		)
	}
	return nil
}

func (c *sshMachine) ensureAPIClient(mc ModelCommand) error {
	if c.sshClient != nil && c.leaderAPI != nil {
		return nil
	}
	conn, err := mc.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	if c.leaderAPI == nil {
		c.leaderAPI = application.NewClient(conn)
	}
	if c.sshClient != nil {
		return nil
	}

	c.sshClient = sshclient.NewFacade(conn)
	c.apiAddr = conn.Addr()
	c.statusClient = apiclient.NewClient(conn, logger)
	return nil
}

func (c *sshMachine) resolveTarget(target string) (*resolvedTarget, error) {
	// If the user specified a leader unit, try to resolve it to the
	// appropriate unit name and override the requested target name.
	resolvedTargetName, err := c.maybeResolveLeaderUnit(target)
	if err != nil {
		return nil, errors.Trace(err)
	}

	out, ok := c.resolveAsAgent(resolvedTargetName)
	if !ok {
		// Not a machine or unit agent target - use directly.
		return out, nil
	}

	getAddress := c.reachableAddressGetter
	if c.proxy {
		// Ideally a reachability scan would be done from the
		// controller's perspective but that isn't possible yet, so
		// fall back to the legacy mode (i.e. use the instance's
		// "private" address).
		//
		// This is in some ways better anyway as a both the external
		// and internal addresses of an instance (if it has both) are
		// likely to be accessible from the controller. With a
		// reachability scan juju ssh could inadvertently end up using
		// the public address when it really should be using the
		// internal/private address.
		logger.Debugf("proxy-ssh enabled so not doing reachability scan")
		getAddress = c.legacyAddressGetter
	}

	return c.resolveWithRetry(*out, getAddress)
}

func (c *sshMachine) resolveAsAgent(target string) (*resolvedTarget, bool) {
	out := new(resolvedTarget)
	out.user, out.entity = splitUserTarget(target)
	isAgent := out.isAgent()

	if !isAgent {
		// Not a machine/unit agent target: resolve - use as-is.
		out.host = out.entity
	} else if out.user == "" {
		out.user = "ubuntu"
	}

	return out, isAgent
}

type addressGetterFunc func(target string) (string, error)

func (c *sshMachine) resolveWithRetry(target resolvedTarget, getAddress addressGetterFunc) (*resolvedTarget, error) {
	// A target may not initially have an address (e.g. the
	// address updater hasn't yet run), so we must do this in
	// a loop.
	out := &target

	callArgs := c.retryStrategy
	callArgs.Func = func() error {
		var err error
		out.host, err = getAddress(out.entity)
		if errors.IsNotFound(err) || params.IsCodeNotFound(err) {
			// Catch issues like passing invalid machine/unit IDs early.
			return errors.Trace(err)
		}

		if err != nil {
			logger.Debugf("getting target %q address(es) failed: %v (retrying)", out.entity, err)
			return errors.Trace(err)
		}

		logger.Debugf("using target %q address %q", out.entity, out.host)
		return nil
	}
	err := retry.Call(callArgs)

	if err != nil {
		err = retry.LastError(err)
		return nil, errors.Trace(err)
	}
	return out, nil
}

// legacyAddressGetter returns the preferred public or private address of the
// given entity (private when c.proxy is true), using the sshClient. Only used
// when the SSHClient API facade v2 is not available or when proxy-ssh is set.
func (c *sshMachine) legacyAddressGetter(entity string) (string, error) {
	if c.proxy {
		return c.sshClient.PrivateAddress(entity)
	}

	return c.sshClient.PublicAddress(entity)
}

// reachableAddressGetter dials all addresses of the given entity, returning the
// first one that succeeds. Only used with SSHClient API facade v2 or later is
// available. It does not try to dial if only one address is available.
func (c *sshMachine) reachableAddressGetter(entity string) (string, error) {
	addresses, err := c.sshClient.AllAddresses(entity)
	if err != nil {
		return "", errors.Trace(err)
	} else if len(addresses) == 0 {
		return "", network.NoAddressError("available")
	} else if len(addresses) == 1 {
		logger.Debugf("Only one SSH address provided (%s), using it without probing", addresses[0])
		return addresses[0], nil
	}
	var publicKeys []string
	if !c.noHostKeyChecks {
		publicKeys, err = c.getKeysWithRetry(entity)
		if err != nil {
			return "", errors.Trace(err)
		}
	}
	var sshPort = defaultSSHPort
	args := c.getArgs()

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-p" && i != len(args)-1 {
			if sshPortNum, err := strconv.Atoi(args[i+1]); err == nil {
				sshPort = sshPortNum
			}
		}
	}

	usable := network.NewMachineHostPorts(sshPort, addresses...).HostPorts().FilterUnusable()
	best, err := c.hostChecker.FindHost(usable, publicKeys)
	if err != nil {
		return "", errors.Trace(err)
	}

	return best.Host(), nil
}

// AllowInterspersedFlags for ssh/scp is set to false so that
// flags after the unit name are passed through to ssh, for eg.
// `juju ssh -v application-name/0 uname -a`.
func (c *sshMachine) AllowInterspersedFlags() bool {
	return false
}

func (c *sshMachine) maybePopulateTargetViaField(target *resolvedTarget, statusGetter func(*client.StatusArgs) (*params.FullStatus, error)) error {
	status, err := statusGetter(nil)
	if err != nil {
		return errors.Trace(err)
	}

	for _, machStatus := range status.Machines {
		if len(machStatus.IPAddresses) == 0 {
			continue
		}

		for _, machIPAddr := range machStatus.IPAddresses {
			if machIPAddr == target.host {
				// We are connecting to a machine. No need to
				// populate the via field.
				return nil
			}
		}

		for _, contStatus := range machStatus.Containers {
			for _, contMachIPAddr := range contStatus.IPAddresses {
				if contMachIPAddr == target.host {
					target.via = &resolvedTarget{
						user: "ubuntu",
						// We have already checked that the host machine has at least one address
						host: machStatus.IPAddresses[0],
					}
					return nil
				}
			}

		}
	}

	return nil
}

func (c *sshMachine) getKeysWithRetry(entity string) ([]string, error) {
	var publicKeys []string
	strategy := c.publicKeyRetryStrategy
	strategy.IsFatalError = func(err error) bool {
		return !errors.Is(err, errors.NotFound)
	}
	strategy.Func = func() error {
		keys, err := c.sshClient.PublicKeys(entity)
		if err != nil {
			return errors.Annotatef(err, "retrieving SSH host keys for %q", entity)
		}
		publicKeys = keys
		return nil
	}
	err := retry.Call(strategy)
	if err != nil {
		return nil, err
	}
	return publicKeys, nil
}

// getJujuExecutable returns the path to the juju
// executable, or an error if it could not be found.
var getJujuExecutable = func() (string, error) {
	return exec.LookPath(os.Args[0])
}

func targetIsAgent(target string) bool {
	return names.IsValidMachine(target) || names.IsValidUnit(target)
}

func splitUserTarget(target string) (string, string) {
	if i := strings.IndexRune(target, '@'); i != -1 {
		return target[:i], target[i+1:]
	}
	return "", target
}

func newKnownHostsBuilder() *knownHostsBuilder {
	return &knownHostsBuilder{
		seen: set.NewStrings(),
	}
}

// knownHostsBuilder supports the construction of a SSH known_hosts file.
type knownHostsBuilder struct {
	lines []string
	seen  set.Strings
}

func (b *knownHostsBuilder) add(host string, keys []string) {
	if b.seen.Contains(host) {
		return
	}
	b.seen.Add(host)
	for _, key := range keys {
		b.lines = append(b.lines, host+" "+key+"\n")
	}
}

func (b *knownHostsBuilder) write(w io.Writer) error {
	bufw := bufio.NewWriter(w)
	for _, line := range b.lines {
		_, err := bufw.WriteString(line)
		if err != nil {
			return errors.Annotate(err, "writing known hosts file")
		}
	}
	_ = bufw.Flush()
	return nil
}

func (b *knownHostsBuilder) size() int {
	return len(b.lines)
}
