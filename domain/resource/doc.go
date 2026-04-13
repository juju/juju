// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package resource provides domain types and logic for managing charm
// resources, including resource definitions, application and unit bindings,
// and repository or uploaded resource state.
//
// Every resource has a type of either a file resource or an OCI image resource.
// Resources are defined in the charm metadata. After deployment, the specific
// resource details are associated with the application deployed from the charm.
//
// Resources can be downloaded from the charm repository or uploaded by the
// juju client. When deploying repository charms, resources are created and
// linked to an application when the application is created, so the logic for
// resource creation belongs to application domain. A local resource is then
// uploaded to the controller. A repository resource is downloaded when the
// resource is first used by the application. When deploying local charms,
// resources are created and uploaded before the application is created.
// During the application creation, it is linked to the previously uploaded
// resource.
//
// For each resource, we keep track of 4 pieces of information:
//
//   - resources are instantiations of the resource definitions from the charm
//     metadata. They hold the origin and revision and can be linked to a
//     blob. They have the state "available".
//
//   - application resources represent which resource revision or blob the
//     application is currently using.
//
//   - unit resources represent which resource instance the unit is currently
//     using.
//
//   - repository resources represent the latest revision available in charm
//     repository. These have the state "potential".
//
// Resources can be refreshed independently of charm revisions, but must exist
// in the current charm metadata used by the application. Refresh from the
// charm repository happens if a new version is available when the `juju
// refresh` command is run or when juju attach-resource is run with a revision
// number or a file to upload.
//
// Uploaded resources, will not have revision, and their potential updated
// revision will not be tracked (as it cannot be known).
//
// Resource are stored based on the type. Files are stored in the object store,
// OCI image resources are stored in the model database.
package resource
