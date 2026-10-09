// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package crossmodel provides connection and controller-location helpers for
// cross-model relation consumers.
//
// Models participating in cross-model relations can migrate between controllers.
// This package supports following controller redirects and persisting the
// controller that now hosts a migrated model.
//
// See github.com/juju/juju/api for controller API connections and
// github.com/juju/juju/core/crossmodel for external controller information.
package crossmodel
