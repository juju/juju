//go:build !dqlite

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"net"

	"github.com/juju/juju/database/dqlite"
)

type Client struct{}

func (c *Client) Cluster(context.Context) ([]dqlite.NodeInfo, error) {
	return nil, nil
}

type YamlNodeStore struct {
	servers []dqlite.NodeInfo
}

func NewYamlNodeStore(_ string) (*YamlNodeStore, error) {
	return &YamlNodeStore{}, nil
}

func (s *YamlNodeStore) Get(context.Context) ([]dqlite.NodeInfo, error) {
	return nil, nil
}

func (s *YamlNodeStore) Set(context.Context, []dqlite.NodeInfo) error {
	return nil
}

// LogFunc is a function that can be used for logging.
type LogFunc = func(LogLevel, string, ...interface{})

// LogLevel defines the logging level.
type LogLevel int

// Available logging levels.
const (
	None LogLevel = iota
	Debug
	Info
	Warn
	Error
)

func (l LogLevel) String() string {
	switch l {
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

type DialFunc func(context.Context, string) (net.Conn, error)
