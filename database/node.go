// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/agent"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/client"
	"github.com/juju/juju/database/dqlite"
)

const (
	dqliteBootstrapBindIP = "127.0.0.1"
	dqliteDataDir         = "dqlite"
	dqlitePort            = 17666
	dqliteClusterFileName = "cluster.yaml"
)

// NodeManager is responsible for interrogating a single Dqlite node,
// and emitting configuration for starting its Dqlite `App` based on
// operational requirements and controller agent config.
type NodeManager struct {
	cfg             agent.Config
	port            int
	logger          Logger
	slowQueryLogger coredatabase.SlowQueryLogger

	dataDir string
}

// NewNodeManager returns a new NodeManager reference
// based on the input agent configuration.
func NewNodeManager(cfg agent.Config, logger Logger, slowQueryLogger coredatabase.SlowQueryLogger) *NodeManager {
	return &NodeManager{
		cfg:             cfg,
		port:            dqlitePort,
		logger:          logger,
		slowQueryLogger: slowQueryLogger,
	}
}

// IsBootstrappedNode returns true if this machine or container was where we
// first bootstrapped Dqlite, and it hasn't been reconfigured since.
// Specifically, whether we are a cluster of one, and bound to the loopback
// IP address.
func (m *NodeManager) IsBootstrappedNode(ctx context.Context) (bool, error) {
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

	return strings.HasPrefix(servers[0].Address, dqliteBootstrapBindIP), nil
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
		dir := filepath.Join(m.cfg.DataDir(), dqliteDataDir)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", errors.Annotatef(err, "creating directory for Dqlite data")
		}
		m.dataDir = dir
	}
	return m.dataDir, nil
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
	if m.cfg.QueryTracingEnabled() {
		return app.WithLogFunc(m.slowQueryLogFunc(m.cfg.QueryTracingThreshold()))
	}
	return app.WithLogFunc(m.appLogFunc)
}

// WithTracingOption returns a Dqlite application Option that will enable
// tracing of Dqlite queries.
func (m *NodeManager) WithTracingOption() app.Option {
	if m.cfg.QueryTracingEnabled() {
		return app.WithTracing(client.LogWarn)
	}
	return app.WithTracing(client.LogNone)
}

// WithLoopbackAddressOption returns a Dqlite application
// Option that will bind Dqlite to the loopback IP.
func (m *NodeManager) WithLoopbackAddressOption() app.Option {
	return m.WithAddressOption(dqliteBootstrapBindIP)
}

// WithAddressOption returns a Dqlite application Option
// for specifying the local address:port to use.
func (m *NodeManager) WithAddressOption(ip string) app.Option {
	return app.WithAddress(fmt.Sprintf("%s:%d", ip, m.port))
}

// WithTLSOption returns a Dqlite application Option for TLS encryption
// of traffic between clients and clustered application nodes.
func (m *NodeManager) WithTLSOption() (app.Option, error) {
	stateInfo, ok := m.cfg.StateServingInfo()
	if !ok {
		return nil, errors.NotSupportedf("Dqlite node initialisation on non-controller machine/container")
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(m.cfg.CACert()))

	controllerCert, err := tls.X509KeyPair([]byte(stateInfo.Cert), []byte(stateInfo.PrivateKey))
	if err != nil {
		return nil, errors.Annotate(err, "parsing controller certificate")
	}

	listen := &tls.Config{
		ClientCAs:    caCertPool,
		Certificates: []tls.Certificate{controllerCert},
	}

	dial := &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{controllerCert},
		// We cannot provide a ServerName value here, so we rely on the
		// server validating the controller's client certificate.
		InsecureSkipVerify: true,
	}

	return app.WithTLS(listen, dial), nil
}

// WithClusterOption returns a Dqlite application Option for initialising
// Dqlite as the member of a cluster with peers representing other controllers.
func (m *NodeManager) WithClusterOption(addrs []string) app.Option {
	peerAddrs := transform.Slice(addrs, func(addr string) string {
		return fmt.Sprintf("%s:%d", addr, m.port)
	})

	m.logger.Debugf("determined Dqlite cluster members: %v", peerAddrs)
	return app.WithCluster(peerAddrs)
}

// nodeClusterStore returns a YamlNodeStore instance based
// on the cluster.yaml file in the Dqlite data directory.
func (m *NodeManager) nodeClusterStore() (*client.YamlNodeStore, error) {
	store, err := client.NewYamlNodeStore(path.Join(m.dataDir, dqliteClusterFileName))
	return store, errors.Annotate(err, "opening Dqlite cluster node store")
}

func (m *NodeManager) slowQueryLogFunc(threshold time.Duration) client.LogFunc {
	return func(level client.LogLevel, msg string, args ...interface{}) {
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

func (m *NodeManager) appLogFunc(level client.LogLevel, msg string, args ...interface{}) {
	actualLevel, known := loggo.ParseLevel(level.String())
	if !known {
		return
	}

	m.logger.Logf(actualLevel, msg, args...)
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
