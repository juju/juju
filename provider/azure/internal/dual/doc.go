// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package dual provides EnvironProvider and storage.Provider implementations
// that switch between one of two providers depending on the content of
// environment configuration. This is to support backwards-compatibility with
// existing deployments of the legacy Azure provider without using a new
// provider name for the current Azure provider.
package dual
