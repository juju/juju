// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The service package provides abstractions and helpers for interacting
// with the services in a host's init system. These can be categorized
// as either high-level or low-level. Only the high-level components are
// intended for external consumption.
//
// The high-level parts are juju-centric, while the low-level ones are
// more generic. The key high-level pieces are the Service and Services
// types and helpers related to them.
package service

// TODO(ericsnow) Move all the internal parts (including the init system
// implementations)into an "internal" subpackage?
// TODO(ericsnow) Factor out the generic parts of the high-level
// components (along with all of the low-level ones) into a separate
// package in the utils repo? Perhaps put it into its own repo?
