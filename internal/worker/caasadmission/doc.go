// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package caasadmission defines the caasadmission worker. This worker is responsible
// for establishing a Kubernetes mutating admission webhook
// (https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)
// so that Juju can process Kubernetes resource object changes. We currently use
// these mutating admission webhooks to watch for resources being created by
// charms in the Kubernetes cluster. Every time we detect such a resource, we
// annotate the new or updated Kubernetes object with a label indicating which
// Juju application was responsible for creating the resource. When the
// application is later removed, we use these labels to find associated
// resources that need to be removed.
package caasadmission
