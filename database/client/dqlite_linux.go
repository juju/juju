//go:build dqlite && linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/canonical/go-dqlite/client"
)

type Client = client.Client

// YamlNodeStore persists a list addresses of dqlite nodes in a YAML file.
type YamlNodeStore = client.YamlNodeStore

// NewYamlNodeStore creates a new YamlNodeStore backed by the given YAML file.
func NewYamlNodeStore(path string) (*YamlNodeStore, error) {
	return client.NewYamlNodeStore(path)
}

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
