// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package recovery defines shared controller recovery values: archive and
// model inventories, typed dump data, replacement observations and reports.
// Validation, decoding and recovery policy belong to the service package;
// database persistence belongs to the state package. Archive filesystem
// handling lives in internal/recovery.
package recovery
