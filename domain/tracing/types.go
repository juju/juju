// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracing

// Endpoints represents the tracing endpoints.
type Endpoints struct {
	HTTP  string
	HTTPS string
	GRPC  string
}
