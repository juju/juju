// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controllernode provides the service that keeps track of controller
// nodes. This is needed when the controller is in high availability mode and
// has multiple communicating instances. It is primarily used to keep track of
// the DQLite nodes located in each controller.
//
// Controller API addresses are published for three routing audiences:
//
//   - Client addresses are used for client discovery and external access.
//   - Agent addresses are used for ordinary agent discovery and reconnect.
//   - Peer addresses are used to reach a specific controller node.
//
// Client and agent addresses may have no controller identity. Such addresses
// are shared endpoints, such as a load-balanced Kubernetes Service, which can
// reach the controller API but not a particular controller. Peer addresses
// always identify the controller they reach and are never shared.
//
// The core rule is:
//
//   - If the consumer needs any healthy controller, include shared addresses.
//   - If the consumer needs a named controller, exclude shared addresses.
//   - Never use shared addresses as a fallback for a named-controller lookup.
package controllernode
