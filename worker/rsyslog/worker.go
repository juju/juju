// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
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
	if version.Current.OS == version.Windows {
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

func (h *RsyslogConfigHandler) TearDown() error {
	if err := os.Remove(h.syslogConfig.ConfigFilePath()); err == nil {
		restartRsyslog()
	}
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
	rsyslogCertPEM, rsyslogKeyPEM, err := cert.NewServer(caCertPEM, caKeyPEM, expiry, nil)
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
