// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package juju is devops distilled.
//
// Project homepage: https://github.com/juju/juju
//
// For more information please refer to the README.md file
// in this directory.

// Request represents the structure of a request.
// - RequestId: (Number) The sequence number of the request.
// - Type: (String) The type of object to act on.
// - Version: (Number) The version of the Type we will be acting on.
// - Id: (String) The ID of a watched object; future implementations may pass one or more IDs as parameters.
// - Request: (String) The action to perform on the object.
// - Params: (JSON) The parameters as a JSON structure; each request implementation should accept bulk requests.

// Response represents the structure of a response.
// - RequestId: (Number) The sequence number of the request.
// - Error: (String) An error message, if any.
// - ErrorCode: (String) The code of the error, if any.
// - Response: (JSON) An optional response as a JSON structure.

package rpc
