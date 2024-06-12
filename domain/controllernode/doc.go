// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controllernode provides the service that keeps track of controller
// nodes. This is needed when the controller is in high availability mode and
// has multiple communicating instances. It is primarily used to keep track of
// the DQLite nodes located in each controller.
package controllernode
