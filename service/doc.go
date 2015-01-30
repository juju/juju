// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The service package provides abstractions and helpers for interacting
// with the juju-managed services in a host's init system.
package service

// TODO(ericsnow) Move all the internal parts (including the init system
// implementations)into an "internal" subpackage?
// TODO(ericsnow) Factor out the generic parts of the high-level
// components (along with all of the low-level ones) into a separate
// package in the utils repo? Perhaps put it into its own repo?
