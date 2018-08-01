// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// RebootInstanceRequest
// you can reboot a running instance by creating a rebootinstancerequest object.
type RebootInstanceRequest struct {
	// Name is the name of the reboot instance request
	Name string `json:"name"`

	// Hard is true if the we are  performing a hard reset
	Hard bool `json:"hard"`

	//Creation_time represents the timestamp of request creation.
	Creation_time string `json:"creation_time"`

	// Error_reason a description of the reason this request entered error state.
	Error_reason string `json:"error_reason,omitempty"`

	// Instance is multipart name of
	// the instance that you want to reboot.
	Instance string `json:"instance"`

	// State is the state of the request.
	State string `json:"state"`

	Instance_id string `json:"instance_id,omitempty"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`

	Request_id string `json:"request_id,omitempty"`
}

// AllRebootInstanceRequests all reboot instance requests that are
// inside the oracle cloud account
type AllRebootInstanceRequests struct {
	Result []RebootInstanceRequest `json:"result,omitempty"`
}
