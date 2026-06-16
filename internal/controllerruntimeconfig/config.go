// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerruntimeconfig

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/agent"
)

const (
	// Filename is the name of the controller runtime configuration file
	// within the controller agent directory.
	Filename = "runtime.conf"
)

// ConfigPath returns the path to the controller runtime configuration
// file in the given controller agent directory.
func ConfigPath(controllerAgentDir string) string {
	return filepath.Join(controllerAgentDir, Filename)
}

// ControllerRuntimeConfig holds the static local startup values required
// by the controller process and Dqlite before the database is available.
//
// The file contains TLS private key material and must be written with
// owner-only permissions (0600). Values in this struct must never be
// logged verbatim.
type ControllerRuntimeConfig struct {
	// ControllerID is the numeric controller agent ID, e.g. "0".
	ControllerID string `yaml:"controller-id"`

	// ControllerUUID is the UUID of the controller.
	ControllerUUID string `yaml:"controller-uuid"`

	// ControllerModelUUID is the UUID of the controller model.
	ControllerModelUUID string `yaml:"controller-model-uuid"`

	// DataDir is the Dqlite data directory root.
	DataDir string `yaml:"data-dir"`

	// LoopbackPreferred controls whether Dqlite should prefer the loopback
	// address instead of the cloud-local TLS-terminated address. This is true
	// for CAAS controllers.
	LoopbackPreferred bool `yaml:"loopback-preferred,omitempty"`

	// LogDir is the controller process log directory.
	LogDir string `yaml:"log-dir"`

	// DqlitePort is the Dqlite application bind/listen port. A value of
	// zero means the controller uses the compiled-in default port.
	DqlitePort int `yaml:"dqlite-port,omitempty"`

	// QueryTracingEnabled controls whether Dqlite query tracing is on.
	QueryTracingEnabled bool `yaml:"query-tracing-enabled"`

	// QueryTracingThreshold is the slow-query threshold for Dqlite. A
	// value of zero means all traced queries are logged.
	QueryTracingThreshold time.Duration `yaml:"query-tracing-threshold"`

	// DqliteBusyTimeout is the SQLite busy timeout for Dqlite.
	DqliteBusyTimeout time.Duration `yaml:"dqlite-busy-timeout"`

	// CACert is the TLS CA certificate PEM block used for Dqlite.
	CACert string `yaml:"ca-cert"`

	// CAPrivateKey is the TLS CA private key PEM block. It is used by
	// the certificate-watcher worker to build the PKI authority at
	// controller startup. This field is sensitive and must not be
	// logged.
	CAPrivateKey string `yaml:"ca-private-key"`

	// ControllerCert is the Dqlite node TLS certificate PEM block.
	ControllerCert string `yaml:"controller-cert"`

	// ControllerPrivateKey is the Dqlite node TLS private key PEM block.
	// This field is sensitive and must not be logged.
	ControllerPrivateKey string `yaml:"controller-private-key"`

	// SystemIdentity is the SSH private key written to the controller
	// system identity file. An empty value means no system identity file
	// is present. This field is sensitive and must not be logged.
	SystemIdentity string `yaml:"system-identity,omitempty"`

	// LogSinkRateLimitBurst is the number of log messages that will be
	// let through before rate limiting begins. A zero value means use
	// the default from apiserver.DefaultLogSinkConfig().
	LogSinkRateLimitBurst int64 `yaml:"log-sink-rate-limit-burst,omitempty"`

	// LogSinkRateLimitRefill is the rate at which log messages are let
	// through once the initial burst is depleted. A zero value means use
	// the default from apiserver.DefaultLogSinkConfig().
	LogSinkRateLimitRefill time.Duration `yaml:"log-sink-rate-limit-refill,omitempty"`

	// APIAddresses holds the API server addresses that the controller uses to
	// connect to other controllers. These are written at bootstrap time and used
	// by the api-remote-caller worker.
	APIAddresses []string `yaml:"api-addresses,omitempty"`
}

// Validate returns an error if any required field is missing or invalid.
func (cfg ControllerRuntimeConfig) Validate() error {
	if !names.IsValidControllerAgent(cfg.ControllerID) {
		return errors.NotValidf("controller ID %q", cfg.ControllerID)
	}
	if !names.IsValidController(cfg.ControllerUUID) {
		return errors.NotValidf("controller UUID %q", cfg.ControllerUUID)
	}
	if !names.IsValidModel(cfg.ControllerModelUUID) {
		return errors.NotValidf("controller model UUID %q", cfg.ControllerModelUUID)
	}
	if cfg.DataDir == "" {
		return errors.NotValidf("empty data-dir")
	}
	if cfg.LogDir == "" {
		return errors.NotValidf("empty log-dir")
	}
	if cfg.DqlitePort != 0 && (cfg.DqlitePort < 1 || cfg.DqlitePort > 65535) {
		return errors.NotValidf("dqlite port %d", cfg.DqlitePort)
	}
	if cfg.QueryTracingThreshold < 0 {
		return errors.NotValidf("negative query-tracing-threshold")
	}
	if cfg.DqliteBusyTimeout < 0 {
		return errors.NotValidf("negative dqlite-busy-timeout")
	}
	if cfg.CACert == "" {
		return errors.NotValidf("empty ca-cert")
	}
	if cfg.CAPrivateKey == "" {
		return errors.NotValidf("empty ca-private-key")
	}
	if cfg.ControllerCert == "" {
		return errors.NotValidf("empty controller-cert")
	}
	if cfg.ControllerPrivateKey == "" {
		return errors.NotValidf("empty controller-private-key")
	}
	for i, addr := range cfg.APIAddresses {
		if addr == "" {
			return errors.NotValidf("empty api-address at index %d", i)
		}
	}
	return nil
}

// ReadControllerRuntimeConfig reads and validates the controller runtime
// configuration from the file at path. It returns an annotated error if
// the file is missing, malformed, or fails validation.
func ReadControllerRuntimeConfig(path string) (ControllerRuntimeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ControllerRuntimeConfig{}, errors.Annotatef(err,
			"reading controller runtime config %q", path)
	}
	var cfg ControllerRuntimeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ControllerRuntimeConfig{}, errors.Annotatef(err,
			"parsing controller runtime config %q", path)
	}
	if err := cfg.Validate(); err != nil {
		return ControllerRuntimeConfig{}, errors.Annotatef(err,
			"validating controller runtime config %q", path)
	}
	return cfg, nil
}

// WriteControllerRuntimeConfig validates cfg and atomically writes it to
// the file at path with 0600 permissions. Parent directories are created
// if they do not exist.
func WriteControllerRuntimeConfig(path string, cfg ControllerRuntimeConfig) error {
	if err := cfg.Validate(); err != nil {
		return errors.Annotate(err, "invalid controller runtime config")
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return errors.Annotate(err, "marshalling controller runtime config")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.Annotatef(err,
			"creating parent directory for controller runtime config %q", path)
	}
	if err := utils.AtomicWriteFile(path, data, 0o600); err != nil {
		return errors.Annotatef(err,
			"writing controller runtime config %q", path)
	}
	return nil
}

// RenderControllerRuntimeConfig validates cfg and marshals it to YAML.
// It is provided for callers that need the raw YAML content, for example
// cloud-init script generation and Kubernetes ConfigMap assembly.
func RenderControllerRuntimeConfig(cfg ControllerRuntimeConfig) ([]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Annotate(err, "invalid controller runtime config")
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "marshalling controller runtime config")
	}
	return data, nil
}

// ParseLogSinkRateLimits reads log-sink rate-limit overrides from the agent
// environment map using the agent.LogSinkRateLimitBurst and
// agent.LogSinkRateLimitRefill keys. Absent values return zeroes, which signal
// "use defaults" in ControllerRuntimeConfig.
func ParseLogSinkRateLimits(agentEnv map[string]string) (burst int64, refill time.Duration, err error) {
	if v := agentEnv[agent.LogSinkRateLimitBurst]; v != "" {
		burst, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, 0, errors.Annotatef(err, "parsing %s", agent.LogSinkRateLimitBurst)
		}
	}
	if v := agentEnv[agent.LogSinkRateLimitRefill]; v != "" {
		refill, err = time.ParseDuration(v)
		if err != nil {
			return 0, 0, errors.Annotatef(err, "parsing %s", agent.LogSinkRateLimitRefill)
		}
	}
	return burst, refill, nil
}
