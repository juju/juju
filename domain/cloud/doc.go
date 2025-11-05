// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cloud contains the service for managing clouds known to Juju.
//
// Clouds represent the infrastructure providers (such as AWS, Azure, GCE,
// MAAS, or LXD) where Juju can deploy and manage workloads. Every model in
// Juju runs on a cloud, and the cloud domain manages cloud definitions,
// credentials, and regions.
//
// # Key Concepts
//
// A cloud consists of:
//   - Cloud definition: type, endpoint, authentication methods
//   - Regions: geographical or logical divisions within a cloud
//   - Credentials: authentication information for accessing the cloud
//   - Capabilities: what features the cloud supports
//
// Clouds can be:
//   - Built-in: pre-configured clouds like AWS, Azure, GCE
//   - Custom: user-added clouds like private MAAS or OpenStack deployments
//   - Controller-specific: clouds available only to a specific controller
//
// # Cloud Lifecycle
//
// Clouds are typically added during controller bootstrap or later via the
// add-cloud command. Cloud definitions can be updated, and unused clouds can
// be removed. Credentials are managed separately and associated with clouds.
package cloud
