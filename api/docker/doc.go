// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package docker provides the public API types for OCI image and registry
// credential representation. These types carry the JSON and YAML struct tags
// required for wire-level serialization so that external clients (e.g. the
// Terraform Juju provider) can marshal resources that the Juju controller can
// deserialize correctly.
package docker
