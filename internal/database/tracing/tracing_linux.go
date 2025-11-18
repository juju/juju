//go:build dqlite && linux

// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracing

import "github.com/canonical/go-dqlite/v3/tracing"

var WithTracer = tracing.WithTracer

type Span = tracing.Span
