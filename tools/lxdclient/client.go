// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	lxdclient "github.com/lxc/lxd/client"
	lxdshared "github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"
	lxdlogger "github.com/lxc/lxd/shared/logger"

	"github.com/juju/juju/container/lxd"
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

// Client is a high-level wrapper around the LXD API client.
type Client struct {
	*lxd.Server
	baseURL string
}

func (c Client) String() string {
	return fmt.Sprintf("Client(%s)", c.baseURL)
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

	var defaultProfile *api.Profile
	var profileETag string
	if cfg.Remote.Protocol != SimplestreamsProtocol {
		if err != nil {
			return nil, errors.Trace(err)
		}

		defaultProfile, profileETag, err = raw.GetProfile("default")
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	newServer, err := lxd.NewServer(raw)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if remoteID == remoteIDForLocal && verifyBridgeConfig {
		// If this is the LXD provider on the localhost, let's do an extra check to
		// make sure the default profile has a correctly configured bridge, and
		// which one is it.
		if err := newServer.VerifyNetworkDevice(defaultProfile, profileETag); err != nil {
			return nil, errors.Trace(err)
		}
	}

	// If the storage API is supported, let's make sure the LXD has a
	// default pool; we'll just use dir backend for now.
	if cfg.Remote.Protocol != SimplestreamsProtocol && newServer.StorageSupported() {
		if err := newServer.EnsureDefaultStorage(defaultProfile, profileETag); err != nil {
			return nil, errors.Trace(err)
		}
	}

	cInfo, err := raw.GetConnectionInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}
	conn := &Client{
		Server:  newServer,
		baseURL: cInfo.URL,
	}
	return conn, nil
}

var newHostClientFromInfo = lxdclient.ConnectLXD
var newSocketClientFromInfo = lxdclient.ConnectLXDUnix

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
func newRawClient(remote Remote) (lxdclient.ContainerServer, error) {
	host := remote.Host

	clientCert := ""
	if remote.Cert != nil && remote.Cert.CertPEM != nil {
		clientCert = string(remote.Cert.CertPEM)
	}

	clientKey := ""
	if remote.Cert != nil && remote.Cert.KeyPEM != nil {
		clientKey = string(remote.Cert.KeyPEM)
	}

	args := &lxdclient.ConnectionArgs{
		TLSClientCert: clientCert,
		TLSClientKey:  clientKey,
		TLSServerCert: remote.ServerPEMCert,
	}

	newClient := newHostClientFromInfo
	if remote.ID() == remoteIDForLocal {
		newClient = newSocketClientFromInfo
		if host == "" {
			host = lxd.SocketPath(nil)
		}
	} else {
		// If it's a URL, leave it alone. Otherwise, we
		// assume it's a hostname, optionally with port.
		url, err := url.Parse(host)
		if err != nil || url.Scheme == "" {
			if _, _, err := net.SplitHostPort(host); err != nil {
				host = net.JoinHostPort(host, lxdshared.DefaultPort)
			}
			host = lxd.EnsureHTTPS(host)
		}

		// Replace the proxy handler with the one managed
		// by Juju's worker/proxyupdater.
		args.Proxy = proxy.DefaultConfig.GetProxy
	}

	logger.Debugf("connecting to LXD server %q: %q", remote.ID(), host)
	client, err := newClient(host, args)
	if err != nil {
		if remote.ID() == remoteIDForLocal {
			err = hoistLocalConnectErr(err)
			return nil, errors.Annotate(err, "can't connect to the local LXD server")
		}
		return nil, errors.Trace(err)
	}

	if remote.Protocol != SimplestreamsProtocol {
		status, _, err := client.GetServer()
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
			return true, "Permission denied, are you in the lxd group?"
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
