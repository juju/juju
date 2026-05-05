// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package provisioning provides a read-side aggregation service for gathering
// all data required to provision a machine instance.
//
// The provisioning domain consolidates queries across machine, application,
// storage, network, model config, and cloud image metadata -- all within a
// single model-database transaction -- to produce a complete
// ProvisioningInfo for a given machine. Controller configuration is fetched
// separately from the controller database.
//
// This domain is intentionally read-only with respect to most tables it
// queries. The only write operation is caching image metadata fetched from
// external simplestreams sources. Other domains (machine, application,
// storage, network) remain the owners of mutations to their respective
// tables.
package provisioner
