// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/lxc/lxd"
	lxdshared "github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"
	lxdlogger "github.com/lxc/lxd/shared/logger"

	"github.com/juju/juju/network"
	"github.com/juju/juju/utils/proxy"
)

var logger = loggo.GetLogger("juju.tools.lxdclient")

// lxdLogProxy proxies LXD's log calls through the juju logger so we can get
// more info about what's going on.
type lxdLogProxy struct {
	logger loggo.Logger
}

func (p *lxdLogProxy) render(msg string, ctx []interface{}) string {
	result := bytes.Buffer{}
	result.WriteString(msg)
	if len(ctx) > 0 {
		result.WriteString(": ")
	}

	/* This is sort of a hack, but it's enforced in the LXD code itself as
	 * well. LXD's logging framework forces us to pass things as "string"
	 * for one argument and then a "context object" as the next argument.
	 * So, we do some basic rendering here to make it look slightly less
	 * ugly.
	 */
	var key string
	for i, entry := range ctx {
		if i != 0 {
			result.WriteString(", ")
		}

		if key == "" {
			key, _ = entry.(string)
		} else {
			result.WriteString(key)
			result.WriteString(": ")
			result.WriteString(fmt.Sprintf("%s", entry))
			key = ""
		}
	}

	return result.String()
}

func (p *lxdLogProxy) Debug(msg string, ctx ...interface{}) {
	// NOTE(axw) the LXD client logs a lot of detail at
	// "debug" level, which is its highest level of logging.
	// We transform this to Trace, to avoid spamming our
	// logs with too much information.
	p.logger.Tracef(p.render(msg, ctx))
}

func (p *lxdLogProxy) Info(msg string, ctx ...interface{}) {
	p.logger.Infof(p.render(msg, ctx))
}

func (p *lxdLogProxy) Warn(msg string, ctx ...interface{}) {
	p.logger.Warningf(p.render(msg, ctx))
}

func (p *lxdLogProxy) Error(msg string, ctx ...interface{}) {
	p.logger.Errorf(p.render(msg, ctx))
}

func (p *lxdLogProxy) Crit(msg string, ctx ...interface{}) {
	p.logger.Criticalf(p.render(msg, ctx))
}

func init() {
	lxdlogger.Log = &lxdLogProxy{loggo.GetLogger("lxd")}
}

const LXDBridgeFile = "/etc/default/lxd-bridge"

// Client is a high-level wrapper around the LXD API client.
type Client struct {
	*configClient
	*certClient
	*profileClient
	*instanceClient
	*imageClient
	*networkClient
	*storageClient
	baseURL                  string
	defaultProfileBridgeName string
}

func (c Client) String() string {
	return fmt.Sprintf("Client(%s)", c.baseURL)
}

func (c Client) DefaultProfileBridgeName() string {
	return c.defaultProfileBridgeName
}

// Connect opens an API connection to LXD and returns a high-level
// Client wrapper around that connection.
func Connect(cfg Config, verifyBridgeConfig bool) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	remoteID := cfg.Remote.ID()

	raw, err := newRawClient(cfg.Remote)
	if err != nil {
		return nil, errors.Trace(err)
	}

	networkAPISupported := false
	storageAPISupported := false
	var defaultProfile *api.Profile
	if cfg.Remote.Protocol != SimplestreamsProtocol {
		status, err := raw.ServerStatus()
		if err != nil {
			return nil, errors.Trace(err)
		}

		if lxdshared.StringInSlice("network", status.APIExtensions) {
			networkAPISupported = true
		}

		if lxdshared.StringInSlice("storage", status.APIExtensions) {
			storageAPISupported = true
		}

		defaultProfile, err = raw.ProfileConfig("default")
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	var bridgeName string
	if remoteID == remoteIDForLocal && verifyBridgeConfig {
		// If this is the LXD provider on the localhost, let's do an extra check to
		// make sure the default profile has a correctly configured bridge, and
		// which one is it.
		bridgeName, err = verifyDefaultProfileBridgeConfig(raw, networkAPISupported, defaultProfile)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	// If the storage API is supported, let's make sure the LXD has a
	// default pool; we'll just use dir backend for now.
	if cfg.Remote.Protocol != SimplestreamsProtocol && storageAPISupported {
		if err := verifyStorageConfiguration(raw, defaultProfile); err != nil {
			return nil, errors.Trace(err)
		}
	}

	conn := &Client{
		configClient:             &configClient{raw},
		certClient:               &certClient{raw},
		profileClient:            &profileClient{raw},
		instanceClient:           &instanceClient{raw, remoteID},
		imageClient:              &imageClient{raw, connectToRaw},
		networkClient:            &networkClient{raw, networkAPISupported},
		storageClient:            &storageClient{raw, storageAPISupported},
		baseURL:                  raw.BaseURL,
		defaultProfileBridgeName: bridgeName,
	}
	return conn, nil
}

var lxdNewClientFromInfo = lxd.NewClientFromInfo

func isSupportedAPIVersion(version string) bool {
	versionParts := strings.Split(version, ".")
	if len(versionParts) < 2 {
		logger.Warningf("LXD API version %q: expected format <major>.<minor>", version)
		return false
	}

	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		logger.Warningf("LXD API version %q: unexpected major number: %v", version, err)
		return false
	}

	if major < 1 {
		logger.Warningf("LXD API version %q: expected major version 1 or later", version)
		return false
	}

	return true
}

// newRawClient connects to the LXD host that is defined in Config.
func newRawClient(remote Remote) (*lxd.Client, error) {
	host := remote.Host

	// LXD socket is different depending on installation method
	// We prefer upstream's preference of snap installed LXD
	debianSocket := filepath.FromSlash("/var/lib/lxd")
	snapSocket := filepath.FromSlash("/var/snap/lxd/common/lxd")
	envSocket := os.Getenv("LXD_DIR")

	// The current lxdclient code ONLY looks at LXD_DIR, as it has no understanding
	// of snaps. This forces us to set LXD_DIR to set the socket path.
	// This should be updated once lxdclient dependency is updated
	if envSocket != "" {
		logger.Debugf("Using environment LXD_DIR as socket path: %q", envSocket)
	} else {
		if _, err := os.Stat(snapSocket); err == nil {
			logger.Debugf("Using LXD snap socket: %q", snapSocket)
			os.Setenv("LXD_DIR", snapSocket)
		} else {
			logger.Debugf("LXD snap socket not found, falling back to debian socket: %q", debianSocket)
			os.Setenv("LXD_DIR", debianSocket)
		}
	}

	if remote.ID() == remoteIDForLocal && host == "" {
		host = "unix://" + lxdshared.VarPath("unix.socket")
	} else {
		// If it's a URL, leave it alone. Otherwise, we
		// assume it's a hostname, optionally with port.
		url, err := url.Parse(host)
		if err != nil || url.Scheme == "" {
			if _, _, err := net.SplitHostPort(host); err != nil {
				host = net.JoinHostPort(host, lxdshared.DefaultPort)
			}
		}
	}
	logger.Debugf("connecting to LXD remote %q: %q", remote.ID(), host)

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

	// Replace the proxy handler with the one managed
	// by Juju's worker/proxyupdater.
	if tr, ok := client.Http.Transport.(*http.Transport); ok {
		tr.Proxy = proxy.DefaultConfig.GetProxy
	}

	if remote.Protocol != SimplestreamsProtocol {
		status, err := client.ServerStatus()
		if err != nil {
			return nil, errors.Trace(err)
		}

		if !isSupportedAPIVersion(status.APIVersion) {
			logger.Warningf("trying to use unsupported LXD API version %q", status.APIVersion)
		} else {
			logger.Infof("using LXD API version %q", status.APIVersion)
		}
	}

	return client, nil
}

// verifyDefaultProfileBridgeConfig takes a LXD API client and extracts the
// network bridge configured on the "default" profile. Additionally, if the
// default bridge bridge is used, its configuration in LXDBridgeFile is also
// inspected to make sure it has a chance to work.
func verifyDefaultProfileBridgeConfig(client *lxd.Client, networkAPISupported bool, config *api.Profile) (string, error) {
	const (
		defaultProfileName = "default"
		configTypeKey      = "type"
		configTypeNic      = "nic"
		configNicTypeKey   = "nictype"
		configBridged      = "bridged"
		configEth0         = "eth0"
		configParentKey    = "parent"
	)

	eth0, ok := config.Devices[configEth0]
	if !ok {
		/* on lxd >= 2.3, there is nothing in the default profile
		 * w.r.t. eth0, because there is no lxdbr0 by default. Let's
		 * handle this case and configure one now.
		 */
		if networkAPISupported {
			if err := CreateDefaultBridgeInDefaultProfile(client); err != nil {
				return "", errors.Annotate(err, "couldn't create default bridge")
			}

			return network.DefaultLXDBridge, nil
		}
		return "", errors.Errorf("unexpected LXD %q profile config without eth0: %+v", defaultProfileName, config)
	} else if networkAPISupported {
		if err := checkBridgeConfig(client, eth0[configParentKey]); err != nil {
			return "", err
		}
	}

	// If eth0 is there, but not with the expected attributes, likewise fail
	// early.
	if eth0[configTypeKey] != configTypeNic || eth0[configNicTypeKey] != configBridged {
		return "", errors.Errorf("unexpected LXD %q profile config: %+v", defaultProfileName, config)
	}

	bridgeName := eth0[configParentKey]
	logger.Infof(`LXD "default" profile uses network bridge %q`, bridgeName)

	if bridgeName != network.DefaultLXDBridge {
		// When the user changed which bridge to use, just return its name and
		// check no further.
		return bridgeName, nil
	}

	/* if the network API is supported, that means the lxd-bridge config
	 * file has been obsoleted so we don't need to check it for correctness
	 */
	if networkAPISupported {
		return bridgeName, nil
	}

	bridgeConfig, err := ioutil.ReadFile(LXDBridgeFile)
	if os.IsNotExist(err) {
		return "", bridgeConfigError("lxdbr0 configured but no config file found at " + LXDBridgeFile)
	} else if err != nil {
		return "", errors.Trace(err)
	}

	if err := checkLXDBridgeConfiguration(string(bridgeConfig)); err != nil {
		return "", errors.Trace(err)
	}

	return bridgeName, nil
}

func bridgeConfigError(err string) error {
	return errors.Errorf(`%s
It looks like your lxdbr0 has not yet been configured. Please configure it via:

	sudo dpkg-reconfigure -p medium lxd

and then bootstrap again.`, err)
}

func ipv6BridgeConfigError(filename string) error {
	return errors.Errorf(`%s has IPv6 enabled.
Juju doesn't currently support IPv6.

IPv6 can be disabled by running:

       sudo dpkg-reconfigure -p medium lxd

and then bootstrap again.`, filename)
}

func checkLXDBridgeConfiguration(conf string) error {
	foundSubnetConfig := false
	for _, line := range strings.Split(conf, "\n") {
		if strings.HasPrefix(line, "USE_LXD_BRIDGE=") {
			b, err := strconv.ParseBool(strings.Trim(line[len("USE_LXD_BRIDGE="):], " \""))
			if err != nil {
				logger.Debugf("couldn't parse bool, skipping USE_LXD_BRIDGE check: %s", err)
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
			if name != network.DefaultLXDBridge {
				return bridgeConfigError(fmt.Sprintf(LXDBridgeFile+" has a bridge named %s, not lxdbr0", name))
			}
		} else if strings.HasPrefix(line, "LXD_IPV4_ADDR=") {
			contents := strings.Trim(line[len("LXD_IPV4_ADDR="):], " \"")
			if len(contents) > 0 {
				foundSubnetConfig = true
			}
		} else if strings.HasPrefix(line, "LXD_IPV6_ADDR=") {
			contents := strings.Trim(line[len("LXD_IPV6_ADDR="):], " \"")
			if len(contents) > 0 {
				return ipv6BridgeConfigError(LXDBridgeFile)
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
		var lxdErr error

		sysErr, ok := u.Err.(*os.SyscallError)
		if ok {
			lxdErr = sysErr.Err
		} else {
			// Try a syscall.Errno as that is what's returned for CentOS
			errno, ok := u.Err.(syscall.Errno)
			if !ok {
				return false, msg
			}
			lxdErr = errno
		}

		switch lxdErr {
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
	$ sudo snap install lxd
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

func verifyStorageConfiguration(client *lxd.Client, defaultProfile *api.Profile) error {
	// If the default profile already has a / device, it's all good
	for _, dev := range defaultProfile.Devices {
		if dev["path"] == "/" {
			return nil
		}
	}

	// Otherwise, we need to add one
	pools, err := client.ListStoragePools()
	if err != nil {
		return err
	}

	poolName := ""
	for _, p := range pools {
		if p.Name == "default" {
			poolName = p.Name
		}
	}

	// use whatever pool there is if there isn't one called "default"
	if poolName == "" && len(pools) > 0 {
		poolName = pools[0].Name
	}

	if poolName == "" {
		poolName = "default"
		err := client.StoragePoolCreate(poolName, "dir", nil)
		if err != nil {
			return err
		}
	}

	if defaultProfile.Devices == nil {
		defaultProfile.Devices = map[string]map[string]string{}
	}

	defaultProfile.Devices["root"] = map[string]string{
		"type": "disk",
		"path": "/",
		"pool": poolName,
	}

	// Now, create a disk device that uses the pool
	return client.PutProfile("default", defaultProfile.ProfilePut)
}
