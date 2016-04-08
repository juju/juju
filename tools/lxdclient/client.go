// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/lxc/lxd"
	lxdshared "github.com/lxc/lxd/shared"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.tools.lxdclient")

const LXDBridgeFile = "/etc/default/lxd-bridge"

// Client is a high-level wrapper around the LXD API client.
type Client struct {
	*serverConfigClient
	*certClient
	*profileClient
	*instanceClient
	*imageClient
	baseURL string
}

func (c Client) String() string {
	return fmt.Sprintf("Client(%s)", c.baseURL)
}

// Connect opens an API connection to LXD and returns a high-level
// Client wrapper around that connection.
func Connect(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	remote := cfg.Remote.ID()

	raw, err := newRawClient(cfg.Remote)
	if err != nil {
		return nil, errors.Trace(err)
	}

	conn := &Client{
		serverConfigClient: &serverConfigClient{raw},
		certClient:         &certClient{raw},
		profileClient:      &profileClient{raw},
		instanceClient:     &instanceClient{raw, remote},
		imageClient:        &imageClient{raw, connectToRaw},
		baseURL:            raw.BaseURL,
	}
	return conn, nil
}

var lxdNewClientFromInfo = lxd.NewClientFromInfo

// newRawClient connects to the LXD host that is defined in Config.
func newRawClient(remote Remote) (*lxd.Client, error) {
	host := remote.Host
	logger.Debugf("connecting to LXD remote %q: %q", remote.ID(), host)

	if remote.ID() == remoteIDForLocal && host == "" {
		host = "unix://" + lxdshared.VarPath("unix.socket")
	} else if !strings.HasPrefix(host, "unix://") {
		_, _, err := net.SplitHostPort(host)
		if err != nil {
			// There is no port here
			host = net.JoinHostPort(host, lxdshared.DefaultPort)
		}
	}

	clientCert := ""
	if remote.Cert != nil && remote.Cert.CertPEM != nil {
		clientCert = string(remote.Cert.CertPEM)
	}

	clientKey := ""
	if remote.Cert != nil && remote.Cert.KeyPEM != nil {
		clientKey = string(remote.Cert.KeyPEM)
	}

	static := false
	public := false
	if remote.Protocol == SimplestreamsProtocol {
		static = true
		public = true
	}

	client, err := lxdNewClientFromInfo(lxd.ConnectInfo{
		Name: remote.ID(),
		RemoteConfig: lxd.RemoteConfig{
			Addr:     host,
			Static:   static,
			Public:   public,
			Protocol: string(remote.Protocol),
		},
		ClientPEMCert: clientCert,
		ClientPEMKey:  clientKey,
		ServerPEMCert: remote.ServerPEMCert,
	})
	if err != nil {
		if remote.ID() == remoteIDForLocal {
			err = hoistLocalConnectErr(err)
			return nil, errors.Annotate(err, "can't connect to the local LXD server")
		}
		return nil, errors.Trace(err)
	}

	/* If this is the LXD provider on the localhost, let's do an extra
	 * check to make sure that lxdbr0 is configured.
	 */
	if remote.ID() == remoteIDForLocal {
		profile, err := client.ProfileConfig("default")
		if err != nil {
			return nil, errors.Trace(err)
		}

		/* If the default profile doesn't have eth0 in it, then the
		 * user has messed with it, so let's just use whatever they set up.
		 *
		 * Otherwise, if it looks like it's pointing at our lxdbr0,
		 * let's check and make sure that is configured.
		 */
		eth0, ok := profile.Devices["eth0"]
		if ok && eth0["type"] == "nic" && eth0["nictype"] == "bridged" && eth0["parent"] == "lxdbr0" {
			conf, err := ioutil.ReadFile(LXDBridgeFile)
			if err != nil {
				if !os.IsNotExist(err) {
					return nil, errors.Trace(err)
				} else {
					return nil, bridgeConfigError("lxdbr0 configured but no config file found at " + LXDBridgeFile)
				}
			}

			if err = checkLXDBridgeConfiguration(string(conf)); err != nil {
				return nil, errors.Trace(err)
			}
		}
	}

	return client, nil
}

func bridgeConfigError(err string) error {
	return errors.Errorf(`%s
It looks like your lxdbr0 has not yet been configured. Please configure it via:

	sudo dpkg-reconfigure -p medium lxd

and then bootstrap again.`, err)
}

func checkLXDBridgeConfiguration(conf string) error {
	foundSubnetConfig := false
	for _, line := range strings.Split(conf, "\n") {
		if strings.HasPrefix(line, "USE_LXD_BRIDGE=") {
			b, err := strconv.ParseBool(strings.Trim(line[len("USE_LXD_BRIDGE="):], " \""))
			if err != nil {
				logger.Warningf("couldn't parse bool, skipping USE_LXD_BRIDGE check: %s", err)
				continue
			}

			if !b {
				return bridgeConfigError("lxdbr0 not enabled but required")
			}
		} else if strings.HasPrefix(line, "LXD_BRIDGE=") {
			name := strings.Trim(line[len("LXD_BRIDGE="):], " \"")
			/* If we're here, we want lxdbr0 to be configured
			 * because the default profile that juju will use says
			 * lxdbr0. So let's fail if it doesn't.
			 */
			if name != "lxdbr0" {
				return bridgeConfigError(fmt.Sprintf(LXDBridgeFile+" has a bridge named %s, not lxdbr0", name))
			}
		} else if strings.HasPrefix(line, "LXD_IPV4_ADDR=") || strings.HasPrefix(line, "LXD_IPV6_ADDR=") {
			contents := strings.Trim(line[len("LXD_IPVN_ADDR="):], " \"")
			if len(contents) > 0 {
				foundSubnetConfig = true
			}
		}
	}

	if !foundSubnetConfig {
		return bridgeConfigError("lxdbr0 has no ipv4 or ipv6 subnet enabled")
	}

	return nil
}

func getMessageFromErr(err error) (bool, string) {
	msg := err.Error()
	t, ok := err.(*url.Error)
	if !ok {
		return false, msg
	}

	u, ok := t.Err.(*net.OpError)
	if !ok {
		return false, msg
	}

	if u.Op == "dial" && u.Net == "unix" {
		errno, ok := u.Err.(*os.SyscallError)
		if !ok {
			return false, msg
		}

		switch errno.Err {
		case syscall.ENOENT:
			return false, "LXD socket not found; is LXD installed & running?"
		case syscall.ECONNREFUSED:
			return true, "LXD refused connections; is LXD running?"
		case syscall.EACCES:
			return true, "Permisson denied, are you in the lxd group?"
		}
	}

	return false, msg
}

func hoistLocalConnectErr(err error) error {

	installed, msg := getMessageFromErr(err)

	configureText := `
Please configure LXD by running:
	$ newgrp lxd
	$ lxd init
`

	installText := `
Please install LXD by running:
	$ sudo apt-get install lxd
and then configure it with:
	$ newgrp lxd
	$ lxd init
`

	hint := installText
	if installed {
		hint = configureText
	}

	return errors.Trace(fmt.Errorf("%s\n%s", msg, hint))
}
