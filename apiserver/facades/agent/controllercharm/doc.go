// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controllercharm defines a facade that the controller charm can use
// to affect changes in the Juju controller. Currently, these changes include:
//   - adding/removing user credentials that a related Prometheus instance can
//     use to access the Juju controller's metrics endpoint.
package controllercharm
