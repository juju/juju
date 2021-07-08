// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import "encoding/json"

// DeployRequest describes a request to transactionally deploy a base bundle
// which may be optionally accompanied by one or more overlay documents.
type DeployRequest struct {
	Options DeployOptions `json:"options"`

	// A set of bundles to be deployed.
	//
	// The first entry in the bundle list must always point to the base
	// bundle; all subsequent target attachments will be interpreted as
	// overlays. The first entry may still contain a multi-doc bundle
	// but the same rules apply (i.e. the first doc will be treated as
	// the base bundle).
	//
	// Clients may optionally generate and append an additional overlay to
	// model any overrides that the user specified when invoking the deploy
	// command (e.g. specify trust, endpoint bindings, deployment targets,
	// number of units etc.).
	Bundles []DeployAttachment `json:"targets"`

	// A list of additional information attachments provided by the client
	// when retrying a deploy request after the controller responds with
	// an ErrAdditionalInformationRequired error.
	Attachments []DeployAttachment `json:"attachments,omitempty"`
}

// DeployMode specifies the deploy mode that the controller should use.
type DeployMode string

const (
	// DeployModeAdditive instructs the controller to perform a diff against
	// the current model and only create entities that are not currently
	// part of the model.
	DeployModeAdditive DeployMode = "additive"
)

// DeployOptions encapsulates the set of options that clients may provide to
// control a server-side deployment.
type DeployOptions struct {
	// Specifies the deploy mode that the controller should use.
	Mode DeployMode `json:"mode"`

	// An optional deployment ID to associate with the entities that will
	// be created as part of this deployment. If omitted, the controller
	// will allocate (and return) a unique deployment ID.
	DeploymentID string `json:"deployment-id,omitempty"`

	// If set to true, the controller will not perform any changes but
	// simply return back the list of operations that would normally
	// perform.
	DryRun bool `json:"dry-run"`

	// Force allows clients to effectively bypass the safety checks
	// performed by the controller and force the deployment to proceed.
	// Warning: using force may cause things to break.
	Force bool `json:"force"`
}

// DeployAttachment describes a generic payload which the client provides as
// part of a deploy request.
type DeployAttachment struct {
	// A URI describing the attachment. The URI schema dicates the
	// attachment type.
	URI string `json:"uri"`

	// When the attachment points to a resource that is local to the client
	// (e.g. a bundle, include file, controller details etc.), Data will
	// contain a serialized version of the attachment contents.
	Data json.RawMessage `json:"data,omitempty"`
}

// DeployResult describes the outcome of a deploy request.
type DeployResult struct {
	// DeploymentID is the ID that the controller associated with this
	// deployment. It is only populated when the deploy request was
	// successful.
	DeploymentID string `json:"deployment-id,omitempty"`

	// DeploymentLog contains the list of changes performed by the controller
	// as part of the deployment. It is only populated when the deploy request
	// was successful.
	DeploymentLog []string `json:"deployment-log,omitempty"`

	// Error is populated when the deploy request failed. Clients should
	// always check if the controller responded with an
	// ErrAdditionalInformationRequired code and retry the original request
	// with the required information attached.
	Error *Error `json:"error,omitempty"`
}
