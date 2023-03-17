// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package controllercharm provides a worker which performs tasks on behalf of the
controller charm. Currently, these tasks include:
- managing user credentials for Prometheus to access Juju metrics.

The chain of command is as follows:
  - The controller charm talks to the introspection worker via the abstract
    domain socket / HTTP server provided by the introspection worker.
  - The introspection worker publishes a pubsub topic, which is received by the
    controllercharm worker.
  - The controllercharm worker calls the controllercharm facade.
*/
package controllercharm
