// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"path/filepath"
)

// TODO(ericsnow) Pull these from elsewhere in juju.
var (
	defaultDataDir        = "/var/lib/juju"
	defaultStartupDir     = "/etc/init"
	defaultLoggingConfDir = "/etc/rsyslog.d"
	defaultLogsDir        = "/var/log/juju"
	defaultSSHDir         = "/home/ubuntu/.ssh"
)

// DefaultPaths is a Paths value using all defaults.
var DefaultPaths = NewPathsDefaults("")

// Paths is an abstraction of FS paths important to juju state.
type Paths interface {
	// DataDir returns the state data directory.
	DataDir() string
	// StartupDir returns the system startup/init directory.
	StartupDir() string
	// LoggingConfDir returns the conf directory for juju's logging system.
	LoggingConfDir() string
	// LogsDir returns the directory where juju stores logs.
	LogsDir() string
	// SSHDir returns the directory used for SSH keys, etc.
	SSHDir() string
}

type paths struct {
	root string

	dataDir        string
	startupDir     string
	loggingConfDir string
	logsDir        string
	sshDir         string
}

// NewPaths returns a new Paths value.  If root is not empty, it is
// treated as a directory and all paths are relative to it.  If root is
// an empty string, all paths (including relative ones) are treated
// as-is.
func NewPaths(root, data, startup, loggingConf, logs, ssh string) *paths {
	p := paths{
		root:           root,
		dataDir:        data,
		startupDir:     startup,
		loggingConfDir: loggingConf,
		logsDir:        logs,
		sshDir:         ssh,
	}
	return &p
}

// NewPathsDefaults returns a new Paths value with defaults set.
func NewPathsDefaults(root string) *paths {
	paths := NewPaths(
		root,
		defaultDataDir,
		defaultStartupDir,
		defaultLoggingConfDir,
		defaultLogsDir,
		defaultSSHDir,
	)
	return paths
}

func (p *paths) DataDir() string {
	return filepath.Join(p.root, p.dataDir)
}

func (p *paths) StartupDir() string {
	return filepath.Join(p.root, p.startupDir)
}

func (p *paths) LoggingConfDir() string {
	return filepath.Join(p.root, p.loggingConfDir)
}

func (p *paths) LogsDir() string {
	return filepath.Join(p.root, p.logsDir)
}

func (p *paths) SSHDir() string {
	return filepath.Join(p.root, p.sshDir)
}
