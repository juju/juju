// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"os"
	"path"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/lxc/lxd"
)

// The LXD repo does not expose any consts for these values.
// TODO(ericsnow) Expose these consts in the LXD repo?
const (
	// see https://github.com/lxc/lxd/blob/master/config.go
	configDefaultFile = "config.yml"
	// see https://github.com/lxc/lxd/blob/master/client.go (readMyCert)
	configCertFile = "client.crt"
	configKeyFile  = "client.key"
)

// Config contains the config values used for a connection to the LXD API.
type Config struct {
	// Namespace identifies the namespace to associate with containers
	// and other resources with which the client interacts. If may be
	// blank.
	Namespace string

	// Dirname identifies where the client will find config files.
	// default: "$HOME/.config/lxc"
	Dirname string

	// Filename is the name of the file in the config directory
	// that holds the client config.
	// default: "config.yaml"
	Filename string

	// Remote identifies the remote server to which the client should
	// connect. For the default "remote" use Local.
	Remote Remote
}

// SetDefaults updates a copy of the config with default values
// where needed.
func (cfg Config) SetDefaults() (Config, error) {
	// We leave a blank namespace alone.

	if cfg.Filename == "" {
		cfg.Filename = configDefaultFile
	}

	if cfg.Dirname == "" {
		// TODO(ericsnow) Switch to filepath as soon as LXD does.
		dirname, _ := path.Split(lxd.ConfigPath(cfg.Filename))
		cfg.Dirname = path.Clean(dirname)
	}

	var err error
	cfg.Remote, err = cfg.Remote.SetDefaults()
	if err != nil {
		return cfg, errors.Trace(err)
	}

	return cfg, nil
}

// Validate checks the client's fields for invalid values.
func (cfg Config) Validate() error {
	// TODO(ericsnow) Check cfg.Namespace (if provided)?

	// TODO(ericsnow) Check cfg.Dirname (if provided)?

	// TODO(ericsnow) Check cfg.Filename (if provided)?

	if err := cfg.Remote.Validate(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// Write writes all the various files for this config.
func (cfg Config) Write() error {
	if err := cfg.Validate(); err != nil {
		return errors.Trace(err)
	}

	origConfigDir := updateLXDVars(cfg.Dirname)
	defer updateLXDVars(origConfigDir)

	if err := cfg.write(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func updateLXDVars(dirname string) string {
	// Change the hard-coded config dir that the raw client uses.
	// TODO(ericsnow) This is exactly what happens in the lxc CLI for
	// the LXD_CONF env var. Once the raw client accepts a path to the
	// config dir we can drop this line.
	// See:
	//   https://github.com/lxc/lxd/blob/master/lxc/main.go
	//   https://github.com/lxc/lxd/issues/1196
	origConfigDir := lxd.ConfigDir
	lxd.ConfigDir = dirname

	return origConfigDir
}

func initializeConfigDir(cfg Config) error {
	logger.Debugf("initializing config dir %q", cfg.Dirname)

	if err := os.MkdirAll(cfg.Dirname, 0775); err != nil {
		return errors.Trace(err)
	}

	// Force the default config to get written. LoadConfig() returns the
	// default config from memory if there isn't a config file on disk.
	// So we load that and then explicitly save it to disk with a call
	// to SaveConfig().
	config, err := lxd.LoadConfig()
	if err != nil {
		return errors.Trace(err)
	}
	if err := lxd.SaveConfig(config); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (cfg Config) write() error {
	// Ensure the initial config is set up.
	if err := initializeConfigDir(cfg); err != nil {
		return errors.Trace(err)
	}

	// Update config.yml, if necessary.
	if err := cfg.writeConfigFile(); err != nil {
		return errors.Trace(err)
	}

	// Write the cert file and key file, if applicable.
	var cert Cert
	if cfg.Remote.Cert != nil {
		cert = *cfg.Remote.Cert
	}
	if err := cert.Validate(); err != nil {
		logger.Debugf("not writing invalid/empty certificate")
	} else {
		if err := cfg.writeCertPEM(cert); err != nil {
			return errors.Trace(err)
		}
		if err := cfg.writeKeyPEM(cert); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (cfg Config) writeConfigFile() error {
	filename := cfg.resolve(cfg.Filename)
	logger.Debugf("writing config file %q", filename)

	// TODO(ericsnow) Cache the low-level config in Config?
	rawCfg, err := lxd.LoadConfig()
	if err != nil {
		return errors.Trace(err)
	}

	if !cfg.Remote.isLocal() {
		// Ensure the remote is set correctly.
		remote := cfg.Remote.Name
		delete(rawCfg.Remotes, remote)
		addr := cfg.Remote.Host
		if err := addServer(rawCfg, remote, addr); err != nil {
			return errors.Trace(err)
		}
	}

	// Write out the updated config, if changed.
	// TODO(ericsnow) Check if changed.
	if err := lxd.SaveConfig(rawCfg); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (cfg Config) writeCertPEM(cert Cert) error {
	filename := cfg.resolve(configCertFile)
	logger.Debugf("writing cert PEM file %q", filename)

	file, err := os.Create(filename)
	if err != nil {
		return errors.Trace(err)
	}
	defer file.Close()

	if err := cert.WriteCertPEM(file); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (cfg Config) writeKeyPEM(cert Cert) error {
	filename := cfg.resolve(configKeyFile)
	logger.Debugf("writing key PEM file %q", filename)

	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return errors.Trace(err)
	}
	defer file.Close()

	if err := cert.WriteKeyPEM(file); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (cfg Config) resolve(file string) string {
	return filepath.Join(cfg.Dirname, file)
}
