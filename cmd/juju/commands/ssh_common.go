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

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"github.com/juju/utils/ssh"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/sshclient"
	"github.com/juju/juju/cmd/modelcmd"
)

// SSHCommon implements functionality shared by sshCommand, SCPCommand
// and DebugHooksCommand.
type SSHCommon struct {
	modelcmd.ModelCommandBase
	proxy           bool
	pty             bool
	noHostKeyChecks bool
	Target          string
	Args            []string
	apiClient       sshAPIClient
	apiAddr         string
	knownHostsPath  string
}

type sshAPIClient interface {
	PublicAddress(target string) (string, error)
	PrivateAddress(target string) (string, error)
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

var sshHostFromTargetAttemptStrategy attemptStarter = attemptStrategy{
	Total: 5 * time.Second,
	Delay: 500 * time.Millisecond,
}

func (c *SSHCommon) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.proxy, "proxy", false, "Proxy through the API server")
	f.BoolVar(&c.pty, "pty", true, "Enable pseudo-tty allocation")
	f.BoolVar(&c.noHostKeyChecks, "no-host-key-checks", false, "Skip host key checking (INSECURE)")
}

// initRun initializes the API connection if required, and determines
// if SSH proxying is required. It must be called at the top of the
// command's Run method.
//
// The apiClient, apiAddr and proxy fields are initialized after this
// call.
func (c *SSHCommon) initRun() error {
	if err := c.ensureAPIClient(); err != nil {
		return errors.Trace(err)
	}
	if proxy, err := c.proxySSH(); err != nil {
		return errors.Trace(err)
	} else {
		c.proxy = proxy
	}
	return nil
}

// cleanupRun removes the temporary SSH known_hosts file (if one was
// created) and closes the API connection. It must be called at the
// end of the command's Run (i.e. as a defer).
func (c *SSHCommon) cleanupRun() {
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
func (c *SSHCommon) getSSHOptions(enablePty bool, targets ...*resolvedTarget) (*ssh.Options, error) {
	var options ssh.Options

	if c.noHostKeyChecks {
		options.SetStrictHostKeyChecking(ssh.StrictHostChecksNo)
		options.SetKnownHostsFile("/dev/null")
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
		} else {
			// If the user's personal known_hosts is used, also use
			// the user's personal StrictHostKeyChecking preferences.
			options.SetStrictHostKeyChecking(ssh.StrictHostChecksUnset)
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

// generateKnownHosts takes the provided targets, retrieves the SSH
// public host keys for them and generates a temporary known_hosts
// file for them.
func (c *SSHCommon) generateKnownHosts(targets []*resolvedTarget) (string, error) {
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
func (c *SSHCommon) proxySSH() (bool, error) {
	if c.proxy {
		// No need to check the API if user explictly requested
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
func (c *SSHCommon) setProxyCommand(options *ssh.Options) error {
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
		"--proxy=false",
		"--no-host-key-checks",
		"--pty=false",
		"ubuntu@"+apiServerHost,
		"-q",
		"nc %h %p",
	)
	return nil
}

func (c *SSHCommon) ensureAPIClient() error {
	if c.apiClient != nil {
		return nil
	}
	return errors.Trace(c.initAPIClient())
}

// initAPIClient initialises the API connection.
func (c *SSHCommon) initAPIClient() error {
	conn, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	c.apiClient = sshclient.NewFacade(conn)
	c.apiAddr = conn.Addr()
	return nil
}

func (c *SSHCommon) resolveTarget(target string) (*resolvedTarget, error) {
	out := new(resolvedTarget)
	out.user, out.entity = splitUserTarget(target)

	// If the target is neither a machine nor a unit assume it's a
	// hostname and try it directly.
	if !targetIsAgent(out.entity) {
		out.host = out.entity
		return out, nil
	}

	if out.user == "" {
		out.user = "ubuntu"
	}

	// A target may not initially have an address (e.g. the
	// address updater hasn't yet run), so we must do this in
	// a loop.
	var err error
	for a := sshHostFromTargetAttemptStrategy.Start(); a.Next(); {
		if c.proxy {
			out.host, err = c.apiClient.PrivateAddress(out.entity)
		} else {
			out.host, err = c.apiClient.PublicAddress(out.entity)
		}
		if err == nil {
			return out, nil
		}
	}
	return nil, err
}

// AllowInterspersedFlags for ssh/scp is set to false so that
// flags after the unit name are passed through to ssh, for eg.
// `juju ssh -v application-name/0 uname -a`.
func (c *SSHCommon) AllowInterspersedFlags() bool {
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
