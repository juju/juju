// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql/driver"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"gopkg.in/yaml.v3"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/client"
	"github.com/juju/juju/internal/database/dqlite"
	dqlitedriver "github.com/juju/juju/internal/database/driver"
)

const (
	dqliteDataDir         = "dqlite"
	dqlitePort            = 17666
	dqliteClusterFileName = "cluster.yaml"
)

// NodeManagerConfig holds the static startup values required by NodeManager
// to configure and operate a Dqlite node. All values are set once at
// construction and treated as immutable thereafter.
type NodeManagerConfig struct {
	// DataDir is the root data directory of the controller agent. Dqlite
	// stores its data under <DataDir>/dqlite.
	DataDir string

	// DqlitePort is the TCP port Dqlite listens on. Zero means use the
	// compiled-in default (17666).
	DqlitePort int

	// QueryTracingEnabled enables per-query tracing in Dqlite, routing
	// log output through the slow-query logger.
	QueryTracingEnabled bool

	// QueryTracingThreshold is the minimum query duration that triggers
	// a slow-query log entry. Only relevant when QueryTracingEnabled is true.
	QueryTracingThreshold time.Duration

	// DqliteBusyTimeout is the duration Dqlite waits when a table is
	// locked before returning SQLITE_BUSY. Zero disables the timeout.
	DqliteBusyTimeout time.Duration

	// CACert is the PEM-encoded CA certificate used to verify Dqlite
	// peer TLS connections.
	CACert string

	// ControllerCert is the PEM-encoded controller TLS certificate
	// presented to Dqlite peers.
	ControllerCert string

	// ControllerPrivateKey is the PEM-encoded private key corresponding
	// to ControllerCert.
	ControllerPrivateKey string
}

// NodeManager is responsible for interrogating a single Dqlite node,
// and emitting configuration for starting its Dqlite `App` based on
// operational requirements and controller agent config.
type NodeManager struct {
	cfg             NodeManagerConfig
	port            int
	logger          logger.Logger
	slowQueryLogger coredatabase.SlowQueryLogger

	dataDir string
}

// NewNodeManager returns a new NodeManager reference
// based on the input controller runtime configuration.
func NewNodeManager(cfg NodeManagerConfig, logger logger.Logger, slowQueryLogger coredatabase.SlowQueryLogger) *NodeManager {
	m := &NodeManager{
		cfg:             cfg,
		port:            dqlitePort,
		logger:          logger,
		slowQueryLogger: slowQueryLogger,
	}
	if cfg.DqlitePort != 0 {
		m.port = cfg.DqlitePort
	}
	return m
}

// IsLoopbackBound returns true if we are a cluster of one,
// and bound to the loopback IP address.
func (m *NodeManager) IsLoopbackBound(ctx context.Context) (bool, error) {
	extant, err := m.IsExistingNode()
	if err != nil {
		return false, errors.Annotate(err, "determining existing Dqlite node")
	}
	if !extant {
		return false, nil
	}

	servers, err := m.ClusterServers(ctx)
	if err != nil {
		return false, errors.Trace(err)
	}

	if len(servers) != 1 {
		return false, nil
	}

	host, _, err := net.SplitHostPort(servers[0].Address)
	if err != nil {
		return false, errors.Annotate(err, "parsing Dqlite node address")
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback(), nil
}

// IsExistingNode returns true if this machine or container has
// ever started a Dqlite `App` before. Specifically, this is whether
// the Dqlite data directory is empty.
func (m *NodeManager) IsExistingNode() (bool, error) {
	if _, err := m.EnsureDataDir(); err != nil {
		return false, errors.Annotate(err, "ensuring Dqlite data directory")
	}

	dir, err := os.Open(m.dataDir)
	if err != nil {
		return false, errors.Annotate(err, "opening Dqlite data directory")
	}

	_, err = dir.Readdirnames(1)
	switch err {
	case nil:
		return true, nil
	case io.EOF:
		return false, nil
	default:
		return false, errors.Annotate(err, "reading Dqlite data directory")
	}
}

// EnsureDataDir ensures that a directory for Dqlite data exists at
// a path determined by the agent config, then returns that path.
func (m *NodeManager) EnsureDataDir() (string, error) {
	if m.dataDir == "" {
		dir := filepath.Join(m.cfg.DataDir, dqliteDataDir)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", errors.Annotatef(err, "creating directory for Dqlite data")
		}
		m.dataDir = dir
	}
	return m.dataDir, nil
}

// SetClusterToLocalNode reconfigures the Dqlite cluster so that it has the
// local node as its only member.
// This is intended as a disaster recovery utility, and should only be called:
// 1. At great need.
// 2. With steadfast guarantees of data integrity.
func (m *NodeManager) SetClusterToLocalNode(ctx context.Context) error {
	node, err := m.NodeInfo()
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(m.SetClusterServers(ctx, []dqlite.NodeInfo{node}))
}

// ClusterServers returns the node information for
// Dqlite nodes configured to be in the cluster.
func (m *NodeManager) ClusterServers(ctx context.Context) ([]dqlite.NodeInfo, error) {
	store, err := m.nodeClusterStore()
	if err != nil {
		return nil, errors.Trace(err)
	}
	servers, err := store.Get(ctx)
	return servers, errors.Annotate(err, "retrieving servers from Dqlite node store")
}

// SetClusterServers reconfigures the Dqlite cluster by writing the
// input servers to Dqlite's Raft log and the local node YAML store.
// This should only be called on a stopped Dqlite node.
func (m *NodeManager) SetClusterServers(ctx context.Context, servers []dqlite.NodeInfo) error {
	store, err := m.nodeClusterStore()
	if err != nil {
		return errors.Trace(err)
	}

	if err := dqlite.ReconfigureMembership(m.dataDir, servers); err != nil {
		return errors.Annotate(err, "reconfiguring Dqlite cluster membership")
	}

	return errors.Annotate(store.Set(ctx, servers), "writing servers to Dqlite node store")
}

// NodeInfo reads the local node information file in the Dqlite directory
// and returns the dqlite.NodeInfo represented by its contents.
func (m *NodeManager) NodeInfo() (dqlite.NodeInfo, error) {
	var node dqlite.NodeInfo

	data, err := os.ReadFile(path.Join(m.dataDir, "info.yaml"))
	if err != nil {
		return node, errors.Annotate(err, "reading info.yaml")
	}

	err = yaml.Unmarshal(data, &node)
	return node, errors.Annotate(err, "decoding NodeInfo")
}

// SetNodeInfo rewrites the local node information file in the Dqlite
// data directory, so that it matches the input NodeInfo.
// This should only be called on a stopped Dqlite node.
func (m *NodeManager) SetNodeInfo(server dqlite.NodeInfo) error {
	data, err := yaml.Marshal(server)
	if err != nil {
		return errors.Annotatef(err, "marshalling NodeInfo %#v", server)
	}
	return errors.Annotatef(
		os.WriteFile(path.Join(m.dataDir, "info.yaml"), data, 0600), "writing info.yaml to %s", m.dataDir)
}

// WithLogFuncOption returns a Dqlite application Option that will proxy Dqlite
// log output via this factory's logger where the level is recognised.
func (m *NodeManager) WithLogFuncOption() app.Option {
	if m.cfg.QueryTracingEnabled {
		return app.WithLogFunc(m.slowQueryLogFunc(m.cfg.QueryTracingThreshold))
	}
	return app.WithLogFunc(m.appLogFunc)
}

// WithTracingOption returns a Dqlite application Option that will enable
// tracing of Dqlite queries.
func (m *NodeManager) WithTracingOption() app.Option {
	if m.cfg.QueryTracingEnabled {
		return app.WithTracing(client.LogWarn)
	}
	return app.WithTracing(client.LogNone)
}

// WithBusyTimeoutOption returns a Dqlite application Option that sets
// the busy timeout based on the agent configuration.
func (m *NodeManager) WithBusyTimeoutOption() app.Option {
	return app.WithBusyTimeout(max(m.cfg.DqliteBusyTimeout, 0))
}

// WithAddressOption returns a Dqlite application Option
// for specifying the local address:port to use.
func (m *NodeManager) WithAddressOption(address string) app.Option {
	// dqlite expects an ipv6 address to be in square brackets
	// e.g. [::1]:1234 so we need to use net.JoinHostPort.
	return app.WithAddress(net.JoinHostPort(address, strconv.Itoa(m.port)))
}

// WithTLSOption returns a Dqlite application Option for TLS encryption
// of traffic between clients and clustered application nodes.
func (m *NodeManager) WithTLSOption() (app.Option, error) {
	if m.cfg.ControllerCert == "" {
		return nil, errors.NotSupportedf("Dqlite node initialisation on non-controller machine/container")
	}

	listen, dial, err := dqliteTLSConfig(
		m.cfg.CACert, m.cfg.ControllerCert, m.cfg.ControllerPrivateKey,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return app.WithTLS(listen, dial), nil
}

func dqliteTLSConfig(
	caCertPEM, certPEM, privateKeyPEM string,
) (*tls.Config, *tls.Config, error) {
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(caCertPEM)) {
		return nil, nil, errors.New("failed to append controller CA cert to pool")
	}

	controllerCert, err := tls.X509KeyPair(
		[]byte(certPEM), []byte(privateKeyPEM),
	)
	if err != nil {
		return nil, nil, errors.Annotate(err, "parsing controller certificate")
	}

	x509Cert, err := x509.ParseCertificate(controllerCert.Certificate[0])
	if err != nil {
		return nil, nil, errors.Annotate(err, "parsing controller x509 certificate")
	}
	if len(x509Cert.DNSNames) == 0 {
		return nil, nil, errors.New("controller certificate has no DNS names")
	}

	listen := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{controllerCert},
		RootCAs:      caCertPool,
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
	listen.BuildNameToCertificate()

	dial := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		RootCAs:            caCertPool,
		Certificates:       []tls.Certificate{controllerCert},
		ClientSessionCache: tls.NewLRUClientSessionCache(0),
		ServerName:         x509Cert.DNSNames[0],
	}

	return listen, dial, nil
}

// WithClusterOption returns a Dqlite application Option for initialising
// Dqlite as the member of a cluster with peers representing other controllers.
func (m *NodeManager) WithClusterOption(addrs []string) app.Option {
	peerAddrs := transform.Slice(addrs, func(addr string) string {
		return net.JoinHostPort(addr, strconv.Itoa(m.port))
	})

	m.logger.Debugf(context.TODO(), "determined Dqlite cluster members: %v", peerAddrs)
	return app.WithCluster(peerAddrs)
}

// TLSDialer returns a Dqlite DialFunc that uses TLS encryption
// for traffic between clients and clustered application nodes.
func (m *NodeManager) TLSDialer(ctx context.Context) (client.DialFunc, error) {
	loopbackBound, err := m.IsLoopbackBound(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if loopbackBound {
		return client.DefaultDialFunc, nil
	}

	if m.cfg.ControllerCert == "" {
		return nil, errors.NotSupportedf("Dqlite node initialisation on non-controller machine/container")
	}

	cert, err := tls.X509KeyPair([]byte(m.cfg.ControllerCert), []byte(m.cfg.ControllerPrivateKey))
	if err != nil {
		return nil, errors.Annotate(err, "parsing controller certificate")
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(m.cfg.ControllerCert)) {
		return nil, errors.New("failed to append controller cert to pool")
	}

	return client.DialFuncWithTLS(
		client.DefaultDialFunc,
		app.SimpleDialTLSConfig(cert, pool),
	), nil
}

// DqliteSQLDriver returns a Dqlite SQL driver that can be used to
// connect to the Dqlite cluster. This is a read only connection, which is
// intended to be used for running queries against the Dqlite cluster (REPL).
func (m *NodeManager) DqliteSQLDriver(ctx context.Context) (driver.Driver, error) {
	store, err := m.nodeClusterStore()
	if err != nil {
		return nil, errors.Annotate(err, "opening node cluster store")
	}

	dialer, err := m.TLSDialer(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return dqlitedriver.New(store, dialer)
}

// LeaderClient returns a Dqlite client that is connected to the leader
// of the Dqlite cluster. This client can be used to run queries directly
// against the leader node, which is useful for administrative tasks or
// for running queries that require a consistent view of the data.
func (s *NodeManager) LeaderClient(ctx context.Context) (*client.Client, error) {
	store, err := s.nodeClusterStore()
	if err != nil {
		return nil, errors.Trace(err)
	}

	dialer, err := s.TLSDialer(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cli, err := client.FindLeader(ctx, store, client.WithDialFunc(dialer))
	if err != nil {
		return nil, errors.Annotate(err, "finding Dqlite leader")
	}
	return cli, nil
}

// nodeClusterStore returns a YamlNodeStore instance based
// on the cluster.yaml file in the Dqlite data directory.
func (m *NodeManager) nodeClusterStore() (*client.YamlNodeStore, error) {
	store, err := client.NewYamlNodeStore(path.Join(m.dataDir, dqliteClusterFileName))
	return store, errors.Annotate(err, "opening Dqlite cluster node store")
}

func (m *NodeManager) slowQueryLogFunc(threshold time.Duration) client.LogFunc {
	return func(level client.LogLevel, msg string, args ...any) {
		if level != client.LogWarn {
			m.appLogFunc(level, msg, args...)
			return
		}

		// If we're tracing the dqlite logs we only want to log slow queries
		// and not all the debug messages.
		queryType, duration, stmt := parseSlowQuery(msg, args, threshold)
		switch queryType {
		case slowQuery:
			m.slowQueryLogger.RecordSlowQuery(msg, stmt, args, duration)
		case normalQuery:
			m.appLogFunc(level, msg, args...)
		default:
			// This is a slow query, but we shouldn't report it.
		}
	}
}

func (m *NodeManager) appLogFunc(level client.LogLevel, msg string, args ...any) {
	translatedLevel := logger.TRACE
	switch level {
	case client.LogDebug:
		translatedLevel = logger.DEBUG
	case client.LogInfo:
		translatedLevel = logger.INFO
	case client.LogWarn:
		translatedLevel = logger.WARNING
	case client.LogError:
		translatedLevel = logger.ERROR
	}
	m.logger.Logf(context.TODO(), translatedLevel, logger.Labels{}, msg, args...)
}

// QueryType represents the type of query that is being sent. This simplifies
// the logic for determining if a query is slow or not and if it should be
// reported.
type queryType int

const (
	normalQuery queryType = iota
	slowQuery
	ignoreSlowQuery
)

// This is highly dependent on the format of the log message, which is
// not ideal, but it's the only way to get the query string out of the
// log message. This potentially breaks if the dqlite library changes the
// format of the log message. It would be better if the dqlite library
// provided a way to get traces from a request that wasn't tied to the logging
// system.
//
// The timed queries logged to the tracing request are for the whole time the
// query is being processed. This includes the network time, along with the
// time performing the sqlite query. If the node is sensitive to latency, then
// it will show up here, even though the query itself might be fast at the
// sqlite level.
//
// Raw log messages will be in the form:
//
//   - "%.3fs request query: %q"
//   - "%.3fs request exec: %q"
//   - "%.3fs request prepared: %q"
//
// It is expected that each log message will have 2 arguments, the first being
// the duration of the query in seconds as a float64. The second being the query
// performed as a string.
func parseSlowQuery(msg string, args []any, slowQueryThreshold time.Duration) (queryType, float64, string) {
	if len(args) != 2 {
		return normalQuery, 0, ""
	}

	// We're not a slow query if the message doesn't match the expected format.
	if !strings.HasPrefix(msg, "%.3fs request ") {
		return normalQuery, 0, ""
	}

	// Validate that the first argument is a float64.
	var duration float64
	switch t := args[0].(type) {
	case float64:
		duration = t
	default:
		return normalQuery, 0, ""
	}

	var stmt string
	switch t := args[1].(type) {
	case string:
		stmt = t
	default:
		return normalQuery, 0, ""
	}

	if duration >= slowQueryThreshold.Seconds() {
		return slowQuery, duration, stmt
	}

	return ignoreSlowQuery, duration, stmt
}
