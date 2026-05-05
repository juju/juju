// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package environs provides abstractions for cloud infrastructure management.
//
// These abstractions enable Juju to manage infrastructure across different
// clouds (AWS, Azure, OpenStack, LXD, etc.), with supporting functionality
// for cloud image metadata and simplestreams discovery. Each cloud type has
// a provider implementation that creates Environ instances. Each Environ
// instance represents a Juju environment on a specific cloud instance and
// provides operations for instance lifecycle management, networking
// configuration, storage provisioning, bootstrapping, etc.
//
// See github.com/juju/juju/environs/config for environment configuration. See
// github.com/juju/juju/environs/bootstrap for controller bootstrapping. See
// github.com/juju/juju/internal/provider for cloud provider implementations.
package environs
