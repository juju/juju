// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package network provides the service for managing juju's networking aspects,
// namely spaces and subnets.
//
// Subnets are always discovered from the underlaying network (as present in
// the provider), and inserted into the DB at bootstrap.
// Spaces are a domain concept, and they are a grouping of subnets which allows
// for easier network (and application/integrations) isolation. Spaces can also
// be discovered from the underlaying provider, only available on MAAS.
package network
