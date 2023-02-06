//go:build dqlite && linux
// +build dqlite,linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/canonical/go-dqlite/client"
)

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
