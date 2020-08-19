// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bufio"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/juju/utils"
	"github.com/juju/utils/ssh"

	"github.com/juju/juju/api/sshclient"
	"github.com/juju/juju/apiserver/params"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	jujussh "github.com/juju/juju/network/ssh"
)

// sshMachine implements functionality shared by sshCommand, SCPCommand
// and DebugHooksCommand.
type sshMachine struct {
	modelName string

	proxy           bool
	noHostKeyChecks bool
	target          string
	args            []string
	apiClient       sshAPIClient
	apiAddr         string
	knownHostsPath  string
	hostChecker     jujussh.ReachableChecker
	forceAPIv1      bool
}

const jujuSSHClientForceAPIv1 = "JUJU_SSHCLIENT_API_V1"

type sshAPIClient interface {
	BestAPIVersion() int
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

// attemptStarter is an interface corresponding to utils.AttemptStrategy
//
// TODO(katco): 2016-08-09: lp:1611427
type attemptStarter interface {
	Start() attempt
}

type attempt interface {
	Next() bool
}

// TODO(katco): 2016-08-09: lp:1611427
type attemptStrategy utils.AttemptStrategy

func (s attemptStrategy) Start() attempt {
	// TODO(katco): 2016-08-09: lp:1611427
	return utils.AttemptStrategy(s).Start()
}

const (
	// SSHRetryDelay is the time to wait for an SSH connection to be established
	// to a single endpoint of a target.
	SSHRetryDelay = 500 * time.Millisecond

	// SSHTimeout is the time to wait for before giving up trying to establish
	// an SSH connection to a target, after retrying.
	SSHTimeout = 5 * time.Second

	// SSHPort is the TCP port used for SSH connections.
	SSHPort = 22
)

var sshHostFromTargetAttemptStrategy attemptStarter = attemptStrategy{
	Total: SSHTimeout,
	Delay: SSHRetryDelay,
}

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
// timeout of SSHRetryDelay and an overall timout of SSHTimeout
func defaultReachableChecker() jujussh.ReachableChecker {
	return jujussh.NewReachableChecker(&net.Dialer{Timeout: SSHRetryDelay}, SSHTimeout)
}

func (c *sshMachine) setHostChecker(checker jujussh.ReachableChecker) {
	if checker == nil {
		checker = defaultReachableChecker()
	}
	c.hostChecker = checker
}

// initRun initializes the API connection if required, and determines
// if SSH proxying is required. It must be called at the top of the
// command's Run method.
//
// The apiClient, apiAddr and proxy fields are initialized after this call.
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

	// Used mostly for testing, but useful for debugging and/or
	// backwards-compatibility with some scripts.
	c.forceAPIv1 = os.Getenv(jujuSSHClientForceAPIv1) != ""
	return nil
}

// cleanupRun removes the temporary SSH known_hosts file (if one was
// created) and closes the API connection. It must be called at the
// end of the command's Run (i.e. as a defer).
func (c *sshMachine) cleanupRun() {
	if c.knownHostsPath != "" {
		os.Remove(c.knownHostsPath)
		c.knownHostsPath = ""
	}
	if c.apiClient != nil {
		c.apiClient.Close()
		c.apiClient = nil
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
		if err := c.setProxyCommand(&options); err != nil {
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

func (c *sshMachine) copy(ctx Context) error {
	args, targets, err := c.expandSCPArgs(c.getArgs())
	if err != nil {
		return err
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
			keys, err := c.apiClient.PublicKeys(target.entity)
			if err != nil {
				return "", errors.Annotatef(err, "retrieving SSH host keys for %q", target.entity)
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

	f, err := ioutil.TempFile("", "ssh_known_hosts")
	if err != nil {
		return "", errors.Annotate(err, "creating known hosts file")
	}
	defer f.Close()
	c.knownHostsPath = f.Name() // Record for later deletion
	if knownHosts.write(f); err != nil {
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
	proxy, err := c.apiClient.Proxy()
	if err != nil {
		return false, errors.Trace(err)
	}
	logger.Debugf("proxy-ssh is %v", proxy)
	return proxy, nil
}

// setProxyCommand sets the proxy command option.
func (c *sshMachine) setProxyCommand(options *ssh.Options) error {
	apiServerHost, _, err := net.SplitHostPort(c.apiAddr)
	if err != nil {
		return errors.Errorf("failed to get proxy address: %v", err)
	}
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
	options.SetProxyCommand(
		juju, "ssh",
		"--model="+c.modelName,
		"--proxy=false",
		"--no-host-key-checks",
		"--pty=false",
		"ubuntu@"+apiServerHost,
		"-q",
		"nc %h %p",
	)
	return nil
}

func (c *sshMachine) ensureAPIClient(mc ModelCommand) error {
	if c.apiClient != nil {
		return nil
	}
	return errors.Trace(c.initAPIClient(mc))
}

// initAPIClient initialises the API connection.
func (c *sshMachine) initAPIClient(mc ModelCommand) error {
	conn, err := mc.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	c.apiClient = sshclient.NewFacade(conn)
	c.apiAddr = conn.Addr()
	return nil
}

func (c *sshMachine) resolveTarget(target string) (*resolvedTarget, error) {
	out, ok := c.resolveAsAgent(target)
	if !ok {
		// Not a machine or unit agent target - use directly.
		return out, nil
	}

	getAddress := c.reachableAddressGetter
	if c.apiClient.BestAPIVersion() < 2 || c.forceAPIv1 {
		logger.Debugf("using legacy SSHClient API v1: no support for AllAddresses()")
		getAddress = c.legacyAddressGetter
	} else if c.proxy {
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
	var err error
	out := &target
	for a := sshHostFromTargetAttemptStrategy.Start(); a.Next(); {
		out.host, err = getAddress(out.entity)
		if errors.IsNotFound(err) || params.IsCodeNotFound(err) {
			// Catch issues like passing invalid machine/unit IDs early.
			return nil, errors.Trace(err)
		}

		if err != nil {
			logger.Debugf("getting target %q address(es) failed: %v (retrying)", out.entity, err)
			continue
		}

		logger.Debugf("using target %q address %q", out.entity, out.host)
		return out, nil
	}

	return nil, errors.Trace(err)
}

// legacyAddressGetter returns the preferred public or private address of the
// given entity (private when c.proxy is true), using the apiClient. Only used
// when the SSHClient API facade v2 is not available or when proxy-ssh is set.
func (c *sshMachine) legacyAddressGetter(entity string) (string, error) {
	if c.proxy {
		return c.apiClient.PrivateAddress(entity)
	}

	return c.apiClient.PublicAddress(entity)
}

// reachableAddressGetter dials all addresses of the given entity, returning the
// first one that succeeds. Only used with SSHClient API facade v2 or later is
// available. It does not try to dial if only one address is available.
func (c *sshMachine) reachableAddressGetter(entity string) (string, error) {
	addresses, err := c.apiClient.AllAddresses(entity)
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
		publicKeys, err = c.apiClient.PublicKeys(entity)
		if err != nil {
			return "", errors.Annotatef(err, "retrieving SSH host keys for %q", entity)
		}
	}

	usable := corenetwork.NewMachineHostPorts(SSHPort, addresses...).HostPorts().FilterUnusable()
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
	bufw.Flush()
	return nil
}

func (b *knownHostsBuilder) size() int {
	return len(b.lines)
}
