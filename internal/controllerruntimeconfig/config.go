// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerruntimeconfig

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/semversion"
)

const (
	// Filename is the name of the controller runtime configuration file
	// within the controller agent directory.
	Filename = "runtime.conf"

	// FileNameBootstrapParams is the name of the bootstrap parameters file.
	// It is written by cloud-init into the snap staging directory and copied
	// to $SNAP_COMMON/bootstrap-params by jujud init.
	FileNameBootstrapParams = "bootstrap-params"
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

	// APIPort is the controller API server listen port.
	APIPort int `yaml:"api-port"`

	// AgentPassword is the controller agent API password used for local worker
	// startup before agent.conf is consulted. This field is sensitive and must
	// not be logged.
	AgentPassword string `yaml:"agent-password"`

	// LoggingConfig is the persisted controller logging override used by the
	// controller logger worker.
	LoggingConfig string `yaml:"logging-config,omitempty"`

	// LoggingOverride is the persisted controller-local logging override used
	// at startup instead of the agent config environment value.
	LoggingOverride string `yaml:"logging-override,omitempty"`

	// LokiEndpoint is the Loki push API endpoint the controller logrouter
	// should forward logs to. Empty means logs are sent through logsink.
	LokiEndpoint string `yaml:"lokiendpoint,omitempty"`

	// LokiCACert is the CA certificate used to validate the Loki endpoint.
	LokiCACert string `yaml:"lokicacert,omitempty"`

	// LokiInsecureSkipVerify controls whether TLS validation is disabled
	// for the Loki endpoint. A nil value means the default (verify
	// enabled) is in effect.
	LokiInsecureSkipVerify *bool `yaml:"lokiinsecureskipverify,omitempty"`

	// LokiOrgID is the organization/tenant ID for multi-tenant Loki
	// deployments. Empty means no X-Scope-OrgID header is sent.
	LokiOrgID string `yaml:"lokiorgid,omitempty"`

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

	// UpgradedToVersionNum records the Juju version that the controller has
	// most recently completed upgrade steps to. A zero value means the
	// controller has not yet completed any upgrade steps.
	UpgradedToVersionNum semversion.Number `yaml:"upgraded-to-version,omitempty"`

	// AgentLogfileMaxSizeMB is the maximum size in MB of the agent log file
	// before rotation. A zero value means use the compiled-in default (240).
	AgentLogfileMaxSizeMB int `yaml:"agent-logfile-max-size-mb,omitempty"`

	// AgentLogfileMaxBackups is the maximum number of rotated agent log files
	// to retain. A zero value means use the compiled-in default (2).
	AgentLogfileMaxBackups int `yaml:"agent-logfile-max-backups,omitempty"`

	// CharmRevisionUpdateInterval overrides the default charm revision
	// update interval for testing. An empty value means use the default
	// (24h).
	CharmRevisionUpdateInterval string `yaml:"charm-revision-update-interval,omitempty"`

	// SocketDir is the directory for group-accessible Unix sockets
	// (control.socket and configchange.socket). When set, socket paths
	// are derived from this directory instead of DataDir. The directory
	// must be owned by root:juju with mode 0750.
	SocketDir string `yaml:"socket-dir,omitempty"`

	// SharedAgentDir is the directory for charm-written configuration
	// files (controller.conf). When set, the controller.conf path is
	// derived from this directory instead of
	// DataDir/agents/controller-<id>/. The directory must be owned by
	// root:juju with mode 0750.
	SharedAgentDir string `yaml:"shared-agent-dir,omitempty"`
}

// UpgradedToVersion returns the Juju version that the controller has most
// recently completed upgrade steps to. It implements the upgrade.Version
// interface so ControllerRuntimeConfig can be passed to internalupgrade.NewLock.
func (cfg ControllerRuntimeConfig) UpgradedToVersion() semversion.Number {
	return cfg.UpgradedToVersionNum
}

// EffectiveSocketDir returns SocketDir when set, otherwise falls back to DataDir.
// This is the single point of truth for resolving the directory where
// control.socket and configchange.socket are created.
func (cfg ControllerRuntimeConfig) EffectiveSocketDir() string {
	if cfg.SocketDir != "" {
		return cfg.SocketDir
	}
	return cfg.DataDir
}

// EffectiveSharedAgentDir returns SharedAgentDir when set, otherwise falls
// back to DataDir. This is the single point of truth for resolving the
// directory where controller.conf is read and written.
func (cfg ControllerRuntimeConfig) EffectiveSharedAgentDir() string {
	if cfg.SharedAgentDir != "" {
		return cfg.SharedAgentDir
	}
	return cfg.DataDir
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
	if cfg.SocketDir != "" && !filepath.IsAbs(cfg.SocketDir) {
		return errors.NotValidf("socket-dir %q", cfg.SocketDir)
	}
	if cfg.SharedAgentDir != "" && !filepath.IsAbs(cfg.SharedAgentDir) {
		return errors.NotValidf("shared-agent-dir %q", cfg.SharedAgentDir)
	}
	if cfg.LogDir == "" {
		return errors.NotValidf("empty log-dir")
	}
	if cfg.APIPort < 1 || cfg.APIPort > 65535 {
		return errors.NotValidf("api port %d", cfg.APIPort)
	}
	if cfg.AgentPassword == "" {
		return errors.NotValidf("empty agent-password")
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

var changeConfigMu sync.Mutex

// Mutator mutates a controller runtime config in place.
type Mutator func(*ControllerRuntimeConfig) error

// ChangeControllerRuntimeConfig reads, mutates, validates, and atomically
// writes the controller runtime config at path.
func ChangeControllerRuntimeConfig(path string, mutate Mutator) error {
	changeConfigMu.Lock()
	defer changeConfigMu.Unlock()

	cfg, err := ReadControllerRuntimeConfig(path)
	if err != nil {
		return errors.Trace(err)
	}
	if err := mutate(&cfg); err != nil {
		return errors.Trace(err)
	}
	if err := WriteControllerRuntimeConfig(path, cfg); err != nil {
		return errors.Annotate(err, "cannot write controller runtime configuration")
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

// Path tokens used in staged runtime configuration. These are replaced by
// jujud init using SNAP_DATA and SNAP_COMMON environment values. They must
// never appear in a final runtime.conf; ReadControllerRuntimeConfig rejects
// any config containing unresolved tokens.
const (
	// TokenSnapData is the staged-path token for $SNAP_DATA.
	TokenSnapData = "@SNAP_DATA@"

	// TokenSnapCommon is the staged-path token for $SNAP_COMMON.
	TokenSnapCommon = "@SNAP_COMMON@"
)

// RenderStagedControllerRuntimeConfig constructs a staged runtime configuration
// template for cloud-init delivery to a strictly-confined snap controller. The
// four snap path fields are replaced with bounded token values that jujud init
// resolves inside the snap context. All other fields are serialized byte-for-
// byte. Validation of the resulting staged YAML is deliberately skipped because
// the token path values are not valid final runtime paths.
//
// Only callers that implement the snap-private cloud-init handoff may use this
// function. All other callers must use RenderControllerRuntimeConfig, which
// requires valid final paths.
func RenderStagedControllerRuntimeConfig(cfg ControllerRuntimeConfig) ([]byte, error) {
	// Replace the four snap-private path fields with bounded tokens. All
	// credential and non-path fields are left byte-for-byte.
	staged := cfg
	staged.DataDir = TokenSnapData
	staged.LogDir = TokenSnapCommon + "/var/log/juju"
	staged.SocketDir = TokenSnapCommon + "/sockets"
	staged.SharedAgentDir = TokenSnapCommon + "/agents/controller-0"

	data, err := yaml.Marshal(staged)
	if err != nil {
		return nil, errors.Annotate(err, "marshalling staged controller runtime config")
	}
	return data, nil
}

// ResolveStagedControllerRuntimeConfig parses a staged runtime configuration
// template and resolves the four documented snap-path token fields using the
// provided snapData and snapCommon values. It returns a validated final
// ControllerRuntimeConfig. An error is returned if any of the four path fields
// contain an unresolved token after substitution, if a token appears in an
// unsupported field, or if the resulting configuration fails Validate.
//
// This function must only be called by jujud init, running inside the snap
// context, where SNAP_DATA and SNAP_COMMON are resolved by snapd.
func ResolveStagedControllerRuntimeConfig(data []byte, snapData, snapCommon string) (ControllerRuntimeConfig, error) {
	if snapData == "" {
		return ControllerRuntimeConfig{}, errors.New("snapData must not be empty")
	}
	if snapCommon == "" {
		return ControllerRuntimeConfig{}, errors.New("snapCommon must not be empty")
	}

	var cfg ControllerRuntimeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ControllerRuntimeConfig{}, errors.Annotate(err, "parsing staged controller runtime config")
	}

	// Verify that no credential or non-path fields contain token text. The
	// bounded contract allows tokens only in the four path fields.
	if err := rejectTokensInNonPathFields(cfg); err != nil {
		return ControllerRuntimeConfig{}, err
	}

	// Resolve the four documented snap-path fields.
	cfg.DataDir = resolveToken(cfg.DataDir, snapData, snapCommon)
	cfg.LogDir = resolveToken(cfg.LogDir, snapData, snapCommon)
	cfg.SocketDir = resolveToken(cfg.SocketDir, snapData, snapCommon)
	cfg.SharedAgentDir = resolveToken(cfg.SharedAgentDir, snapData, snapCommon)

	// After resolution the path fields must not still carry any token text.
	if err := rejectUnresolvedTokens(cfg); err != nil {
		return ControllerRuntimeConfig{}, err
	}

	if err := cfg.Validate(); err != nil {
		return ControllerRuntimeConfig{}, errors.Annotate(err, "validating resolved controller runtime config")
	}
	return cfg, nil
}

// resolveToken replaces token prefixes in s with the corresponding snap path.
func resolveToken(s, snapData, snapCommon string) string {
	s = strings.ReplaceAll(s, TokenSnapData, snapData)
	s = strings.ReplaceAll(s, TokenSnapCommon, snapCommon)
	return s
}

// rejectTokensInNonPathFields returns an error if any field other than the
// four documented snap-path fields contains token text. Credential and
// non-path fields must never be substituted.
func rejectTokensInNonPathFields(cfg ControllerRuntimeConfig) error {
	nonPathFields := map[string]string{
		"agent-password":      cfg.AgentPassword,
		"ca-cert":             cfg.CACert,
		"ca-private-key":      cfg.CAPrivateKey,
		"controller-cert":     cfg.ControllerCert,
		"controller-cert-key": cfg.ControllerPrivateKey,
		"system-identity":     cfg.SystemIdentity,
		"logging-config":      cfg.LoggingConfig,
		"logging-override":    cfg.LoggingOverride,
	}
	for name, val := range nonPathFields {
		if strings.Contains(val, TokenSnapData) || strings.Contains(val, TokenSnapCommon) {
			return errors.Errorf("token found in non-path field %q: tokens are only permitted in path fields", name)
		}
	}
	return nil
}

// rejectUnresolvedTokens returns an error if any path field still contains
// token text after resolution.
func rejectUnresolvedTokens(cfg ControllerRuntimeConfig) error {
	pathFields := map[string]string{
		"data-dir":         cfg.DataDir,
		"log-dir":          cfg.LogDir,
		"socket-dir":       cfg.SocketDir,
		"shared-agent-dir": cfg.SharedAgentDir,
	}
	for name, val := range pathFields {
		if strings.Contains(val, TokenSnapData) || strings.Contains(val, TokenSnapCommon) {
			return errors.Errorf("unresolved token in path field %q after resolution", name)
		}
	}
	return nil
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
