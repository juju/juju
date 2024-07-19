// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package application provides the domain types for an application.
// The application domain is the primary domain for creation and handling of an
// application.
//
// The charm is the stored representation of the application. The application
// is the instance manifest of the charm and the unit is the running instance
// of the application.
//
// Charm types are stored in the charm domain, to ensure that the charm is
// handled correctly and that the charm is correctly represented in the domain.
package application
