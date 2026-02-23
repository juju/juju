// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package providers registers all the available cloud providers with the environs package.
// This is JAAS to determine which providers are available and the schema of their credentials.
// The providers themselves are implemented in the internal/provider/* packages, and this
// package imports those packages to trigger their init() functions, which performs the
// provider registration.
package providers
