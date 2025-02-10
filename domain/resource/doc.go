// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package resource provides the domain types for handling charm resources once
// associated with application. Resource creation and link to an application is
// done at the creation of the application, so the logic for resource creation
// belongs to application domain.
//
// Every resource is either a file resource or an OCI image resource. They are
// defined in the charm metadata. After deployment, the specific resource details
// are associated with the application deployed from the charm.
//
// For each resource, we keep track of 4 pieces of information:
//
//   - resources are instantiations of the resource definitions from the
//     charm metadata. They hold the origin and revision and can be linked to
//     a blob. They have the state "available".
//
//   - application resources represent which resource revision or blob the
//     application is currently using.
//
//   - unit resources represent which resource instance the unit is
//     currently using.
//
//   - repository resources represent the latest revision available in charm
//     repository. These have the state "potential".
//
// Resources can be refreshed independently of charm revisions, but must exist
// in the current charm version used by the application.
//
// Resources can be uploaded by a client. In this case, they will not be
// downloaded from charm repository, will not have revision, and their potential
// updated revision will not be tracked (as it cannot be known).
//
// Resource types are stored in this package to ensure correct handling and
// representation within the domain.
package resource
