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
	"path/filepath"
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
	rsyslogCAKey  string
}

// certPair holds the path and contents for a certificate.
type certPair struct {
	path string
	data string
}

var _ worker.NotifyWatchHandler = (*RsyslogConfigHandler)(nil)

func syslogUser() string {
	var user string
	switch version.Current.OS {
	case version.CentOS:
		user = "root"
	default:
		user = "syslog"
	}

	return user
}

var NewRsyslogConfigWorker = newRsyslogConfigWorker

// newRsyslogConfigWorker returns a worker.Worker that uses
// WatchForRsyslogChanges and updates rsyslog configuration based
// on changes. The worker will remove the configuration file
// on teardown.
func newRsyslogConfigWorker(st *apirsyslog.State, mode RsyslogMode, tag names.Tag, namespace string, stateServerAddrs []string, jujuConfigDir string) (worker.Worker, error) {
	if version.Current.OS == version.Windows && mode == RsyslogModeAccumulate {
		return worker.NewNoOpWorker(), nil
	}
	handler, err := newRsyslogConfigHandler(st, mode, tag, namespace, stateServerAddrs, jujuConfigDir)
	if err != nil {
		return nil, err
	}
	logger.Debugf("starting rsyslog worker mode %v for %q %q", mode, tag, namespace)
	return worker.NewNotifyWorker(handler), nil
}

func newRsyslogConfigHandler(st *apirsyslog.State, mode RsyslogMode, tag names.Tag, namespace string, stateServerAddrs []string, jujuConfigDir string) (*RsyslogConfigHandler, error) {
	if namespace != "" {
		jujuConfigDir += "-" + namespace
	}
	jujuConfigDir = filepath.Join(jujuConfigDir, "rsyslog")
	if err := os.MkdirAll(jujuConfigDir, 0755); err != nil {
		return nil, errors.Trace(err)
	}

	syslogConfig := &syslog.SyslogConfig{
		LogFileName:          tag.String(),
		LogDir:               logDir,
		JujuConfigDir:        jujuConfigDir,
		Port:                 0,
		Namespace:            namespace,
		StateServerAddresses: stateServerAddrs,
	}
	if mode == RsyslogModeAccumulate {
		syslog.NewAccumulateConfig(syslogConfig)
	} else {
		syslog.NewForwardConfig(syslogConfig)
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
		if err := h.ensureCA(); err != nil {
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
		namespace := h.syslogConfig.Namespace
		if namespace != "" {
			namespace = "-" + namespace
		}
		logTag := "juju" + namespace + "-" + h.tag.String()
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

func (h *RsyslogConfigHandler) Handle(_ <-chan struct{}) error {
	cfg, err := h.st.GetRsyslogConfig(h.tag.String())
	if err != nil {
		return errors.Annotate(err, "cannot get environ config")
	}

	rsyslogCACert := cfg.CACert
	if rsyslogCACert == "" {
		return nil
	}

	rsyslogCAKey := cfg.CAKey
	if rsyslogCAKey == "" {
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
		rsyslogCertPEM, rsyslogKeyPEM, err := h.rsyslogServerCerts(rsyslogCACert, rsyslogCAKey)
		if err != nil {
			return errors.Trace(err)
		}

		if err := writeCertificates([]certPair{
			{h.syslogConfig.ServerCertPath(), rsyslogCertPEM},
			{h.syslogConfig.ServerKeyPath(), rsyslogKeyPEM},
			{h.syslogConfig.CACertPath(), rsyslogCACert},
		}); err != nil {
			return errors.Trace(err)
		}

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
	h.rsyslogCAKey = rsyslogCAKey
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

// ensureCA ensures that a CA certificate and key exist in state,
// to be picked up by all rsyslog workers in the environment.
func (h *RsyslogConfigHandler) ensureCA() error {
	// We never write the CA key to local disk, so
	// we must check state to know whether or not
	// we need to generate new certs and keys.
	cfg, err := h.st.GetRsyslogConfig(h.tag.String())
	if err != nil {
		return errors.Annotate(err, "cannot get environ config")
	}
	if cfg.CACert != "" && cfg.CAKey != "" {
		return nil
	}

	// Generate a new CA and server cert/key pairs, and
	// publish to state. Rsyslog workers will observe
	// this and generate certificates and keys for
	// rsyslog in response.
	expiry := time.Now().UTC().AddDate(10, 0, 0)
	caCertPEM, caKeyPEM, err := cert.NewCA("rsyslog", expiry)
	if err != nil {
		return err
	}
	return h.st.SetRsyslogCert(caCertPEM, caKeyPEM)
}

// writeCertificates persists any certPair to disk. If any
// of the write attempts fail it will return an error immediately.
// It is up to the caller to ensure the order of pairs represents
// a suitable order in the case of failue.
func writeCertificates(pairs []certPair) error {
	// Files must be chowned to syslog:adm.
	syslogUid, syslogGid, err := lookupUser(syslogUser())
	if err != nil {
		return err
	}

	for _, pair := range pairs {
		if err := writeFileAtomic(pair.path, []byte(pair.data), 0600, syslogUid, syslogGid); err != nil {
			return err
		}
	}
	return nil
}

// rsyslogServerCerts generates new certificates for RsyslogConfigHandler
// using the provider caCert and caKey. This is used during the setup of the
// rsyslog worker as well as when handling any changes to the rsyslog configuration,
// usually adding and removing of state machines through ensure-availability.
func (h *RsyslogConfigHandler) rsyslogServerCerts(caCert, caKey string) (string, string, error) {
	if caCert == "" {
		return "", "", errors.New("CACert is not set")
	}
	if caKey == "" {
		return "", "", errors.New("CAKey is not set")
	}

	expiry := time.Now().UTC().AddDate(10, 0, 0)
	// Add rsyslog servers in the subjectAltName so we can
	// successfully validate when connectiong via SSL
	hosts, err := h.rsyslogHosts()
	if err != nil {
		return "", "", err
	}
	// Add local IPs to SAN. When connecting via IP address,
	// the client will validate the server against any IP in
	// the subjectAltName. We add all local ips to make sure
	// this does not cause an error
	ips, err := localIPS()
	if err != nil {
		return "", "", err
	}
	hosts = append(hosts, ips...)
	return cert.NewServer(caCert, caKey, expiry, hosts)
}

// ensureLogrotate ensures that the logrotate
// configuration file and logrotate helper script
// exist in the log directory and creates them if they do not.
func (h *RsyslogConfigHandler) ensureLogrotate() error {
	// Files must be chowned to syslog
	syslogUid, syslogGid, err := lookupUser(syslogUser())
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
