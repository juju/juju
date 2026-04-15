// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package oci implements the Oracle Cloud Infrastructure (OCI) provider.
//
// OCI provider operations map Juju model behavior to OCI compute, networking,
// storage, and identity APIs.
//
// The provider is registered with the environs registry as "oci".
// See github.com/juju/juju/internal/provider/common for functionality common
// to all providers. See github.com/juju/juju/internal/provider for other
// providers. See github.com/juju/juju/environs for provider interfaces. See
// the sections below for oci package-wide behavior boundaries.
//
// # How the oci provider differs from other providers
//
//   - Identity and tenancy model:
//     OCI uses OCI HTTP-signature credentials and compartment-scoped APIs.
//
//   - Provider-managed networking:
//     OCI creates and manages model VCN, route, gateway, security-list, and
//     subnet resources.
//
//   - Image/shape coupling:
//     OCI machine creation requires selecting an image first, then choosing
//     from shapes that support that image. A shape is OCI hardware sizing.
//
//   - Storage lifecycle handling:
//     OCI storage lifecycle is handled through OCI block-volume APIs.
//
//   - Placement semantics:
//     OCI availability domains are first-class placement inputs.
//
// # Configuration
//
// The following provider-specific model config keys are defined in provider.go:
//
//   - compartment-id: (REQUIRED) the OCID of the compartment in which Juju
//     has access to create resources. Validated at provider open time.
//   - address-space: (OPTIONAL) the CIDR block to use when creating the model
//     VCN and subnets. Must have a /8 to /16 prefix length. Defaults to
//     10.0.0.0/16 if not supplied.
//
// Auth types supported: httpsig. Credentials require user OCID, tenancy OCID,
// PEM private key, key fingerprint, and optional pass-phrase. Credentials can
// be detected from the OCI CLI config file (~/.oci/config).
//
// # Networking
//
// Unlike providers that can attach to existing model networking, OCI
// provisions and manages the model network layout. Juju creates a VCN for the
// model, then creates route/security resources and one subnet per
// availability domain from the configured (or default) address space.
//
// # Instances and images
//
// Bootstrap and deployment are based on OCI compute instances and OCI image
// selection. Juju machine requests are translated into OCI instance creation
// parameters.
//
// # Storage
//
// Storage uses OCI block-volume APIs.
//
// # Regions and Availability Zones
//
// Region definitions come from the OCI Go SDK:
//
//	github.com/oracle/oci-go-sdk/master/common/region.go
//
// OCI Availability Domains map to availability zone behavior for placement.
//
// # Maintainer notes
//
// When changing OCI behavior, preserve these invariants:
//
//   - validate configuration early
//   - keep provider-managed networking
//   - keep instance and storage lifecycle logic aligned with OCI semantics
//   - do not assume generic cloud defaults
//   - preserve image-to-supported-shape constraints
//
// Small changes in bootstrap, networking, or storage can have broad effects
// on provider behavior.
package oci
