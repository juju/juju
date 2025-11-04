// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The storage provisioning domain is responsible for managing all storage
// requirements for a model making sure that storage is provisioned and attached
// to the correct entities in the model.
//
// The domain mostly exists to provide information into the storage provisioning
// worker.
//
// # Provisioning Scope
// For legacy reasons Juju manages the provisioning of storage bases on one of
// two scpes. The first scope is "model" which indicates that the provisioning
// logic for a given storage entity runs within the scope of the model. This can
// be thought of as running on the controller itself.
//
// The second scope is "machine" where the provisiong of the storage happens
// on the actual machine where the storage is to be used. This second scope only
// ever applies to models which deploy machines to run units. A common case for
// machine provisioned storage entities is filesystems where the paritioning and
// making of the filesystem needs to be run from the actual machine itself.
//
// A third scope has been introduced into our modeling of storage that is not
// yet in use but it exists as a place holder to support future directions and
// also requirements that exist today. This scope is "external" where the
// provisioner of the storage does not exist within Juju and is performed by
// another process outside the normal control loop of Juju. An example of this
// today would be Kubernetes where storage is provisioned and handled by
// the statefulset controller.
//
// A forward looking design would would need "external" provisiong scope  to
// move away from a singular storage provisioning worker to one where each
// provider of storage registers themselves into the controller as being able
// to provision a set of the storage requirements for a model. This design every
// storage provider would become an "external" provisioner.
package storageprovisioning
