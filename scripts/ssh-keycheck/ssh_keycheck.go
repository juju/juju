// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path"
	"strings"
	"time"

	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v2"
	"golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/network"
	jujussh "github.com/juju/juju/internal/network/ssh"
)

func knownHostFilename() string {
	usr, err := user.Current()
	if err != nil {
		panic(fmt.Sprintf("unable to find current user: %v", err))
	}
	return path.Join(usr.HomeDir, ".ssh", "known_hosts")
}

// Juju reports the files in /etc/ssh/ssh_host_key_*_key.pub, so they are all
// in AuthorizedKey format.
func getKnownHostKeys(fname string) []string {
	f, err := os.Open(fname)
	if err != nil {
		panic(fmt.Sprintf("unable to read known-hosts file: %q %v", fname, err))
	}
	defer func() { _ = f.Close() }()

	content, err := io.ReadAll(f)
	if err != nil {
		panic(fmt.Sprintf("failed while reading known-hosts file: %q %v", fname, err))
	}
	pubKeys := make([]string, 0)
	for len(content) > 0 {
		// marker, hosts, pubkey, comment, rest, err
		_, _, pubkey, _, remaining, err := ssh.ParseKnownHosts(content)
		if err != nil {
			panic(fmt.Sprintf("failed while parsing known hosts: %q %v", fname, err))
		}
		content = remaining
		// We convert the "known_hosts" format into AuthorizedKeys format to
		// match what Juju records.
		pubKeys = append(pubKeys, string(ssh.MarshalAuthorizedKey(pubkey)))
	}
	return pubKeys
}

var logger = loggo.GetLogger("juju.ssh_keyscan")

func main() {
	var verbose bool
	var dialTimeout int = 500
	var waitTimeout int = 5000
	var hostFile string
	gnuflag.BoolVar(&verbose, "v", false, "dump debugging information")
	gnuflag.IntVar(&dialTimeout, "dial-timeout", 500, "time to try a single connection (in milliseconds)")
	gnuflag.IntVar(&waitTimeout, "wait-timeout", 5000, "overall time to wait for answers (in milliseconds)")
	gnuflag.StringVar(&hostFile, "known-hosts", knownHostFilename(), "point to an alternate known-hosts file")
	gnuflag.Parse(true)
	if verbose {
		_ = loggo.ConfigureLoggers(`<root>=DEBUG`)
	}
	args := gnuflag.Args()
	pubKeys := getKnownHostKeys(hostFile)

	dialAddresses := make(network.HostPorts, 0, len(args))
	for _, arg := range args {
		if strings.Index(arg, ":") < 0 {
			// Not valid for IPv6, but good enough for testing
			arg = arg + ":22"
		}
		hp, err := network.ParseMachineHostPort(arg)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "invalid host:port value: %v\n%v\n", arg, err)
			return
		}
		dialAddresses = append(dialAddresses, *hp)
	}

	addrs := make([]string, len(dialAddresses))
	for i, addr := range dialAddresses {
		addrs[i] = network.DialAddress(addr)
	}
	logger.Infof("host ports: %v\n", addrs)
	logger.Infof("found %d known hosts\n", len(pubKeys))
	logger.Debugf("known hosts: %v\n", pubKeys)

	dialer := &net.Dialer{Timeout: time.Duration(dialTimeout) * time.Millisecond}
	checker := jujussh.NewReachableChecker(dialer, time.Duration(waitTimeout)*time.Millisecond)
	found, err := checker.FindHost(dialAddresses, pubKeys)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "could not find valid host: %v\n", err)
		return
	}
	fmt.Printf("%s\n", network.DialAddress(found))
}
