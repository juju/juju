//go:build dqlite && linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/canonical/go-dqlite/v2/client"
)

// Client is a dqlite client that can be used to connect to a dqlite cluster.
type Client = client.Client

// FindLeader finds the leader of a dqlite cluster using the provided NodeStore.
func FindLeader(ctx context.Context, store NodeStore, opts ...client.Option) (*Client, error) {
	return client.FindLeader(ctx, store, opts...)
}

// WithDialFunc sets a custom dial function for creating the client network
// connection.
func WithDialFunc(dial DialFunc) client.Option {
	return client.WithDialFunc(dial)
}

// YamlNodeStore persists a list addresses of dqlite nodes in a YAML file.
type YamlNodeStore = client.YamlNodeStore

// DefaultDialFunc is the default dial function, which can handle plain TCP and
// Unix socket endpoints. You can customize it with WithDialFunc()
func DefaultDialFunc(ctx context.Context, address string) (net.Conn, error) {
	return client.DefaultDialFunc(ctx, address)
}

// DialFunc is a function that can be used to establish a network connection.
type DialFunc = client.DialFunc

// DialFuncWithTLS returns a dial function that uses TLS encryption.
//
// The given dial function will be used to establish the network connection,
// and the given TLS config will be used for encryption.
func DialFuncWithTLS(dial DialFunc, config *tls.Config) DialFunc {
	return client.DialFuncWithTLS(dial, config)
}

// NewYamlNodeStore creates a new YamlNodeStore backed by the given YAML file.
func NewYamlNodeStore(path string) (*YamlNodeStore, error) {
	return client.NewYamlNodeStore(path)
}

// NodeStore is a store of dqlite node addresses.
type NodeStore = client.NodeStore

// LogFunc is a function that can be used for logging.
type LogFunc = client.LogFunc

// LogLevel defines the logging level.
type LogLevel = client.LogLevel

// Available logging levels.
const (
	LogNone  = client.LogNone
	LogDebug = client.LogDebug
	LogInfo  = client.LogInfo
	LogWarn  = client.LogWarn
	LogError = client.LogError
)
