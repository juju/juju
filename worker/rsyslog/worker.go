// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"time"

	"github.com/errgo/errgo"
	"github.com/loggo/loggo"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/log/syslog"
	"launchpad.net/juju-core/names"
	apirsyslog "launchpad.net/juju-core/state/api/rsyslog"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.worker.rsyslog")

const rsyslogConfDir = "/etc/rsyslog.d"

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
	st                     *apirsyslog.State
	mode                   RsyslogMode
	syslogConfig           *syslog.SyslogConfig
	rsyslogConfPath        string
	rsyslogGnutlsInstalled bool
}

var _ worker.NotifyWatchHandler = (*RsyslogConfigHandler)(nil)

// NewRsyslogConfigWorker returns a worker.Worker that watches
// for config changes and updates rsyslog configuration based
// on changes.
// NewRsyslogConfigWorker also returns the path to the rsyslog
// configuration file that is written, so that the agent may
// remove it on shutdown.
func NewRsyslogConfigWorker(st *apirsyslog.State, config agent.Config, mode RsyslogMode) (worker.Worker, error) {
	namespace := config.Value(agent.Namespace)
	var syslogConfig *syslog.SyslogConfig
	if mode == RsyslogModeAccumulate {
		syslogConfig = syslog.NewAccumulateConfig(config.Tag(), 0, namespace)
	} else {
		addr, err := config.APIAddresses()
		if err != nil {
			return nil, err
		}
		syslogConfig = syslog.NewForwardConfig(config.Tag(), 0, namespace, addr)
	}

	// Historically only machine-0 includes the namespace in the log
	// dir/file; for backwards compatibility we continue the tradition.
	if config.Tag() != "machine-0" {
		namespace = ""
	}
	kind, err := names.TagKind(config.Tag())
	if err != nil {
		return nil, err
	}
	if kind == names.MachineTagKind {
		if namespace == "" {
			syslogConfig.ConfigFileName = "25-juju.conf"
		} else {
			syslogConfig.ConfigFileName = fmt.Sprintf("25-juju-%s.conf", namespace)
		}
	} else {
		syslogConfig.ConfigFileName = fmt.Sprintf("26-juju-%s.conf", config.Tag())
	}

	if namespace != "" {
		syslogConfig.LogDir += "-" + namespace
	}
	worker := worker.NewNotifyWorker(&RsyslogConfigHandler{
		st:           st,
		mode:         mode,
		syslogConfig: syslogConfig,
	})
	return worker, nil
}

func (h *RsyslogConfigHandler) SetUp() (watcher.NotifyWatcher, error) {
	if h.mode == RsyslogModeAccumulate {
		if err := h.ensureCertificates(); err != nil {
			return nil, errgo.Annotate(err, "failed to write rsyslog certificates")
		}
	}
	return h.st.WatchForEnvironConfigChanges()
}

func (h *RsyslogConfigHandler) TearDown() error {
	if err := os.Remove(h.syslogConfig.ConfigFilePath()); err == nil {
		syslog.Restart()
	}
	return nil
}

func (h *RsyslogConfigHandler) Handle() error {
	if !h.rsyslogGnutlsInstalled {
		if err := utils.AptGetInstall("rsyslog-gnutls"); err != nil {
			// apt-get may fail if another process has the lock,
			// so keep we'll just exit and try again next time.
			return errgo.Annotate(err, "cannot install rsyslog-gnutls")
		}
		h.rsyslogGnutlsInstalled = true
	}
	cfg, err := h.st.EnvironConfig()
	if err != nil {
		return errgo.Annotate(err, "cannot get environ config")
	}
	h.syslogConfig.Port = cfg.SyslogPort()
	rsyslogCACert := cfg.RsyslogCACert()
	if rsyslogCACert == nil {
		return nil
	}
	if h.mode == RsyslogModeForwarding {
		h.syslogConfig.TLSCACertPath = h.caCertPath()
		if err := writeFileAtomic(h.syslogConfig.TLSCACertPath, rsyslogCACert, 0644, 0, 0); err != nil {
			return err
		}
	}
	data, err := h.syslogConfig.Render()
	if err != nil {
		return errgo.Annotate(err, "failed to render rsyslog configuration file")
	}
	if err := writeFileAtomic(h.syslogConfig.ConfigFilePath(), []byte(data), 0644, 0, 0); err != nil {
		return errgo.Annotate(err, "failed to write rsyslog configuration file")
	}
	logger.Debugf("Reloading rsyslog configuration")
	if err := syslog.Restart(); err != nil {
		logger.Errorf("failed to reload rsyslog configuration")
		return err
	}
	return nil
}

func (h *RsyslogConfigHandler) caCertPath() string {
	return filepath.Join(h.syslogConfig.LogDir, "ca-cert.pem")
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
	caCertPEM := h.caCertPath()
	if _, err := os.Stat(caCertPEM); err == nil {
		return nil
	}

	// Files must be chowned to syslog:adm.
	syslogUser, err := user.Lookup("syslog")
	if err != nil {
		return err
	}
	syslogUid, err := strconv.Atoi(syslogUser.Uid)
	if err != nil {
		return err
	}
	syslogGid, err := strconv.Atoi(syslogUser.Gid)
	if err != nil {
		return err
	}

	// Generate a new CA and server cert/key pairs.
	// The CA key will be discarded after the server
	// cert has been generated.
	expiry := time.Now().UTC().AddDate(10, 0, 0)
	caCertPEMBytes, caKeyPEMBytes, err := cert.NewCA("rsyslog", expiry)
	if err != nil {
		return err
	}
	rsyslogCertPEMBytes, rsyslogKeyPEMBytes, err := cert.NewServer(caCertPEMBytes, caKeyPEMBytes, expiry, nil)
	if err != nil {
		return err
	}

	// Update the environment config with the CA cert,
	// so clients can configure rsyslog.
	if err := h.st.SetRsyslogCert(caCertPEMBytes); err != nil {
		return err
	}

	// Write the certificates and key. The CA certificate must be written last for idempotency.
	h.syslogConfig.TLSCACertPath = caCertPEM
	h.syslogConfig.TLSCertPath = filepath.Join(h.syslogConfig.LogDir, "rsyslog-cert.pem")
	h.syslogConfig.TLSKeyPath = filepath.Join(h.syslogConfig.LogDir, "rsyslog-key.pem")
	for _, pair := range []struct {
		path string
		data []byte
	}{
		{h.syslogConfig.TLSCertPath, rsyslogCertPEMBytes},
		{h.syslogConfig.TLSKeyPath, rsyslogKeyPEMBytes},
		{h.syslogConfig.TLSCACertPath, caCertPEMBytes},
	} {
		if err := writeFileAtomic(pair.path, pair.data, 0600, syslogUid, syslogGid); err != nil {
			return err
		}
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode, uid, gid int) error {
	temp := path + ".temp"
	if err := ioutil.WriteFile(temp, data, mode); err != nil {
		return err
	}
	if uid != -1 {
		if err := os.Chown(temp, uid, gid); err != nil {
			return err
		}
	}
	return os.Rename(temp, path)
}
