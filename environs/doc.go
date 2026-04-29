// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package environs provides abstractions for cloud infrastructure management.
//
// These abstractions enable Juju to manage infrastructure across different
// clouds (AWS, Azure, OpenStack, LXD, etc.), with supporting functionality
// for cloud image metadata and simplestreams discovery. Each cloud type has
// a provider implementation that registers via RegisterProvider and implements
// either EnvironProvider (for all providers) or CloudEnvironProvider (for
// traditional clouds) to create Environ instances. A Juju environment on a
// specific cloud instance is represented by an Environ, which provides
// operations for instance lifecycle management, networking configuration,
// storage provisioning, bootstrapping, etc.
//
// See github.com/juju/juju/environs/config for environment configuration. See
// github.com/juju/juju/environs/bootstrap for controller bootstrapping. See
// github.com/juju/juju/internal/provider for cloud provider implementations.
package environs
