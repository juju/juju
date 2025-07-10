//go:build !dqlite

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/juju/juju/internal/database/dqlite"
	"github.com/juju/juju/internal/errors"
)

// Client is a dqlite client that can be used when dqlite is not available.
type Client struct{}

// Cluster returns an empty cluster and no error, as dqlite is not available.
func (c *Client) Cluster(context.Context) ([]dqlite.NodeInfo, error) {
	return nil, nil
}

// Leader returns information about the current leader, if any.
func (c *Client) Leader(ctx context.Context) (*dqlite.NodeInfo, error) {
	return nil, nil
}

// FindLeader returns no leader and no error, as dqlite is not available.
func FindLeader(ctx context.Context, store NodeStore, opts ...Option) (*Client, error) {
	return nil, nil
}

// WithDialFunc sets a custom dial function for creating the client network
// connection.
func WithDialFunc(dial DialFunc) Option {
	return Option{}
}

type Option struct{}

type YamlNodeStore struct{}

func NewYamlNodeStore(_ string) (*YamlNodeStore, error) {
	return &YamlNodeStore{}, nil
}

func (s *YamlNodeStore) Get(context.Context) ([]dqlite.NodeInfo, error) {
	return nil, nil
}

func (s *YamlNodeStore) Set(context.Context, []dqlite.NodeInfo) error {
	return nil
}

type NodeStore interface {
	Get(context.Context) ([]dqlite.NodeInfo, error)
	Set(context.Context, []dqlite.NodeInfo) error
}

// LogFunc is a function that can be used for logging.
type LogFunc = func(LogLevel, string, ...interface{})

// LogLevel defines the logging level.
type LogLevel int

// Available logging levels.
const (
	LogNone LogLevel = iota
	LogDebug
	LogInfo
	LogWarn
	LogError
)

func (l LogLevel) String() string {
	switch l {
	case LogDebug:
		return "DEBUG"
	case LogInfo:
		return "INFO"
	case LogWarn:
		return "WARN"
	case LogError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

type DialFunc func(context.Context, string) (net.Conn, error)

// DefaultDialFunc is the default dial function, which can handle plain TCP and
// Unix socket endpoints. You can customize it with WithDialFunc()
func DefaultDialFunc(ctx context.Context, address string) (net.Conn, error) {
	return nil, errors.Errorf("dqlite is not available")
}

// DialFuncWithTLS returns a dial function that uses TLS encryption.
//
// The given dial function will be used to establish the network connection,
// and the given TLS config will be used for encryption.
func DialFuncWithTLS(dial DialFunc, config *tls.Config) DialFunc {
	return nil
}
