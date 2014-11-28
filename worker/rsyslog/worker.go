// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	rsyslog "github.com/juju/syslog"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	apirsyslog "github.com/juju/juju/api/rsyslog"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/utils/syslog"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.rsyslog")

var (
	rsyslogConfDir = "/etc/rsyslog.d"
	logDir         = agent.DefaultLogDir
	syslogTargets  = []*rsyslog.Writer{}
)

// RsyslogMode describes how to configure rsyslog.
type RsyslogMode int

const (
	RsyslogModeInvalid RsyslogMode = iota
	// RsyslogModeForwarding is the mode in which
	// rsyslog will be configured to forward logging
	// to state servers.
	RsyslogModeForwarding
	// RsyslogModeAccumulate is the mode in which
	// rsyslog will be configured to accumulate logging
	// from other machines into an "all-machines.log".
	RsyslogModeAccumulate
)

// RsyslogConfigHandler implements worker.NotifyWatchHandler, watching
// environment configuration changes and generating new rsyslog
// configuration.
type RsyslogConfigHandler struct {
	st              *apirsyslog.State
	mode            RsyslogMode
	syslogConfig    *syslog.SyslogConfig
	rsyslogConfPath string
	tag             names.Tag
	// We store the syslog-port and rsyslog-ca-cert
	// values after writing the rsyslog configuration,
	// so we can decide whether a change has occurred.
	syslogPort    int
	rsyslogCACert string
}

var _ worker.NotifyWatchHandler = (*RsyslogConfigHandler)(nil)

// NewRsyslogConfigWorker returns a worker.Worker that uses
// WatchForRsyslogChanges and updates rsyslog configuration based
// on changes. The worker will remove the configuration file
// on teardown.
func NewRsyslogConfigWorker(st *apirsyslog.State, mode RsyslogMode, tag names.Tag, namespace string, stateServerAddrs []string) (worker.Worker, error) {
	if version.Current.OS == version.Windows && mode == RsyslogModeAccumulate {
		return worker.NewNoOpWorker(), nil
	}
	handler, err := newRsyslogConfigHandler(st, mode, tag, namespace, stateServerAddrs)
	if err != nil {
		return nil, err
	}
	logger.Debugf("starting rsyslog worker mode %v for %q %q", mode, tag, namespace)
	return worker.NewNotifyWorker(handler), nil
}

func newRsyslogConfigHandler(st *apirsyslog.State, mode RsyslogMode, tag names.Tag, namespace string, stateServerAddrs []string) (*RsyslogConfigHandler, error) {
	var syslogConfig *syslog.SyslogConfig
	if mode == RsyslogModeAccumulate {
		syslogConfig = syslog.NewAccumulateConfig(tag.String(), logDir, 0, namespace, stateServerAddrs)
	} else {
		syslogConfig = syslog.NewForwardConfig(tag.String(), logDir, 0, namespace, stateServerAddrs)
	}

	// Historically only machine-0 includes the namespace in the log
	// dir/file; for backwards compatibility we continue the tradition.
	if tag != names.NewMachineTag("0") {
		namespace = ""
	}
	switch tag := tag.(type) {
	case names.MachineTag:
		if namespace == "" {
			syslogConfig.ConfigFileName = "25-juju.conf"
		} else {
			syslogConfig.ConfigFileName = fmt.Sprintf("25-juju-%s.conf", namespace)
		}
	default:
		syslogConfig.ConfigFileName = fmt.Sprintf("26-juju-%s.conf", tag)
	}

	syslogConfig.ConfigDir = rsyslogConfDir
	syslogConfig.LogDir = logDir
	if namespace != "" {
		syslogConfig.LogDir += "-" + namespace
	}
	return &RsyslogConfigHandler{
		st:           st,
		mode:         mode,
		syslogConfig: syslogConfig,
		tag:          tag,
	}, nil
}

func (h *RsyslogConfigHandler) SetUp() (watcher.NotifyWatcher, error) {
	if h.mode == RsyslogModeAccumulate {
		if err := h.ensureCertificates(); err != nil {
			return nil, errors.Annotate(err, "failed to write rsyslog certificates")
		}

		if err := h.ensureLogrotate(); err != nil {
			return nil, errors.Annotate(err, "failed to write rsyslog logrotate scripts")
		}

	}
	return h.st.WatchForRsyslogChanges(h.tag.String())
}

var restartRsyslog = syslog.Restart
var dialSyslog = rsyslog.Dial

func (h *RsyslogConfigHandler) TearDown() error {
	if err := os.Remove(h.syslogConfig.ConfigFilePath()); err == nil {
		restartRsyslog()
	}
	return nil
}

// composeTLS generates a new client certificate for connecting to the rsyslog server.
// We explicitly set the ServerName field, this ensures that even if we are connecting
// via an IP address and are using an old certificate (pre 1.20.9), we can still
// successfully connect.
func (h *RsyslogConfigHandler) composeTLS(caCert string) (*tls.Config, error) {
	cert := x509.NewCertPool()
	ok := cert.AppendCertsFromPEM([]byte(caCert))
	if !ok {
		return nil, errors.Errorf("Failed to parse rsyslog root certificate")
	}
	return &tls.Config{
		RootCAs:    cert,
		ServerName: "juju-rsyslog",
	}, nil
}

func (h *RsyslogConfigHandler) replaceRemoteLogger(caCert string) error {
	tlsConf, err := h.composeTLS(caCert)
	if err != nil {
		return err
	}

	var newLoggers []*rsyslog.Writer
	var wrapLoggers []io.Writer
	for _, j := range h.syslogConfig.StateServerAddresses {
		host, _, err := net.SplitHostPort(j)
		if err != nil {
			// No port was found
			host = j
		}
		target := fmt.Sprintf("%s:%d", host, h.syslogConfig.Port)
		logTag := "juju" + h.syslogConfig.Namespace + "-" + h.tag.String()
		logger.Debugf("making syslog connection for %q to %s", logTag, target)
		writer, err := dialSyslog("tcp", target, rsyslog.LOG_DEBUG, logTag, tlsConf)
		if err != nil {
			return err
		}
		wrapLoggers = append(wrapLoggers, writer)
		newLoggers = append(newLoggers, writer)
	}
	wapper := io.MultiWriter(wrapLoggers...)
	writer := loggo.NewSimpleWriter(wapper, &loggo.DefaultFormatter{})

	loggo.RemoveWriter("syslog")
	err = loggo.RegisterWriter("syslog", writer, loggo.TRACE)
	if err != nil {
		return err
	}

	// Close old targets
	for _, j := range syslogTargets {
		if err := j.Close(); err != nil {
			logger.Warningf("Failed to close syslog writer: %s", err)
		}
	}
	// record new targets
	syslogTargets = newLoggers
	return nil
}

func (h *RsyslogConfigHandler) Handle() error {
	// TODO(dfc)
	cfg, err := h.st.GetRsyslogConfig(h.tag.String())
	if err != nil {
		return errors.Annotate(err, "cannot get environ config")
	}
	rsyslogCACert := cfg.CACert
	if rsyslogCACert == "" {
		return nil
	}

	h.syslogConfig.Port = cfg.Port
	if h.mode == RsyslogModeForwarding {
		if err := writeFileAtomic(h.syslogConfig.CACertPath(), []byte(rsyslogCACert), 0644, 0, 0); err != nil {
			return errors.Annotate(err, "cannot write CA certificate")
		}
		if err := h.replaceRemoteLogger(rsyslogCACert); err != nil {
			return err
		}
	} else {
		data, err := h.syslogConfig.Render()
		if err != nil {
			return errors.Annotate(err, "failed to render rsyslog configuration file")
		}
		if err := writeFileAtomic(h.syslogConfig.ConfigFilePath(), []byte(data), 0644, 0, 0); err != nil {
			return errors.Annotate(err, "failed to write rsyslog configuration file")
		}
		logger.Debugf("Reloading rsyslog configuration")
		if err := restartRsyslog(); err != nil {
			logger.Errorf("failed to reload rsyslog configuration")
			return errors.Annotate(err, "cannot restart rsyslog")
		}
	}
	// Record config values so we don't try again.
	// Do this last so we recover from intermittent
	// failures.
	h.syslogPort = cfg.Port
	h.rsyslogCACert = rsyslogCACert
	return nil
}

var lookupUser = func(username string) (uid, gid int, err error) {
	u, err := user.Lookup(username)
	if err != nil {
		return -1, -1, err
	}
	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return -1, -1, err
	}
	gid, err = strconv.Atoi(u.Gid)
	if err != nil {
		return -1, -1, err
	}
	return uid, gid, nil
}

func localIPS() ([]string, error) {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, j := range addrs {
		ip, _, err := net.ParseCIDR(j.String())
		if err != nil {
			return nil, err
		}
		if ip.IsLoopback() {
			continue
		}
		ips = append(ips, ip.String())
	}
	return ips, nil
}

func (h *RsyslogConfigHandler) rsyslogHosts() ([]string, error) {
	var hosts []string
	cfg, err := h.st.GetRsyslogConfig(h.tag.String())
	if err != nil {
		return nil, err
	}
	for _, j := range cfg.HostPorts {
		if j.Value != "" {
			hosts = append(hosts, j.Address.Value)
		}
	}

	// Explicitly add the '*' wildcard host. This will ensure that rsyslog
	// clients will always be able to connect even if their hostnames and/or IPAddresses
	// are changed. This also ensures we can continue to use SSL for our rsyslog connections
	// and we can avoid having to use the skipVerify flag.
	hosts = append(hosts, "*")

	return hosts, nil
}

// ensureCertificates ensures that a CA certificate,
// server certificate, and private key exist in the log
// directory, and writes them if not. The CA certificate
// is entered into the environment configuration to be
// picked up by other agents.
func (h *RsyslogConfigHandler) ensureCertificates() error {
	// We write ca-cert.pem last, after propagating into state.
	// If it's there, then there's nothing to do. Otherwise,
	// start over.
	caCertPEM := h.syslogConfig.CACertPath()
	if _, err := os.Stat(caCertPEM); err == nil {
		return nil
	}

	// Files must be chowned to syslog:adm.
	syslogUid, syslogGid, err := lookupUser("syslog")
	if err != nil {
		return err
	}

	// Generate a new CA and server cert/key pairs.
	// The CA key will be discarded after the server
	// cert has been generated.
	expiry := time.Now().UTC().AddDate(10, 0, 0)
	caCertPEM, caKeyPEM, err := cert.NewCA("rsyslog", expiry)
	if err != nil {
		return err
	}

	// Add rsyslog servers in the subjectAltName so we can
	// successfully validate when connectiong via SSL
	hosts, err := h.rsyslogHosts()
	if err != nil {
		return err
	}
	// Add local IPs to SAN. When connecting via IP address,
	// the client will validate the server against any IP in
	// the subjectAltName. We add all local ips to make sure
	// this does not cause an error
	ips, err := localIPS()
	if err != nil {
		return err
	}
	hosts = append(hosts, ips...)
	rsyslogCertPEM, rsyslogKeyPEM, err := cert.NewServer(caCertPEM, caKeyPEM, expiry, hosts)
	if err != nil {
		return err
	}

	// Update the environment config with the CA cert,
	// so clients can configure rsyslog.
	if err := h.st.SetRsyslogCert(caCertPEM); err != nil {
		return err
	}

	// Write the certificates and key. The CA certificate must be written last for idempotency.
	for _, pair := range []struct {
		path string
		data string
	}{
		{h.syslogConfig.ServerCertPath(), rsyslogCertPEM},
		{h.syslogConfig.ServerKeyPath(), rsyslogKeyPEM},
		{h.syslogConfig.CACertPath(), caCertPEM},
	} {
		if err := writeFileAtomic(pair.path, []byte(pair.data), 0600, syslogUid, syslogGid); err != nil {
			return err
		}
	}
	return nil
}

// ensureLogrotate ensures that the logrotate
// configuration file and logrotate helper script
// exist in the log directory and creates them if they do not.
func (h *RsyslogConfigHandler) ensureLogrotate() error {
	// Files must be chowned to syslog
	syslogUid, syslogGid, err := lookupUser("syslog")
	if err != nil {
		return err
	}

	logrotateConfPath := h.syslogConfig.LogrotateConfPath()
	// check for the logrotate conf
	if _, err := os.Stat(logrotateConfPath); os.IsNotExist(err) {
		logrotateConfFile, err := h.syslogConfig.LogrotateConfFile()
		if err != nil {
			return err
		}
		// create the logrotate conf
		if err := writeFileAtomic(logrotateConfPath, logrotateConfFile, 0600, syslogUid, syslogGid); err != nil {
			return err
		}
	} else {
		return err
	}

	logrotateHelperPath := h.syslogConfig.LogrotateHelperPath()
	// check for the logrotate helper
	if _, err := os.Stat(logrotateHelperPath); os.IsNotExist(err) {
		logrotateHelperFile, err := h.syslogConfig.LogrotateHelperFile()
		if err != nil {
			return err
		}
		// create the logrotate helper
		if err := writeFileAtomic(logrotateHelperPath, logrotateHelperFile, 0700, syslogUid, syslogGid); err != nil {
			return err
		}
	} else {
		return err
	}

	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode, uid, gid int) error {
	chmodAndChown := func(f *os.File) error {
		// f.Chmod() and f.Chown() are not implemented on Windows
		// There is currently no good way of doing file permission
		// management for Windows, directly from Go. The behavior of os.Chmod()
		// is different from its linux implementation.
		if runtime.GOOS == "windows" {
			return nil
		}
		if err := f.Chmod(mode); err != nil {
			return err
		}
		if uid != 0 {
			if err := f.Chown(uid, gid); err != nil {
				return err
			}
		}
		return nil
	}
	return utils.AtomicWriteFileAndChange(path, data, chmodAndChown)
}
