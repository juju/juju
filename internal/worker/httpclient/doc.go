// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package httpclient provides a worker that manages http clients. The
// worker is responsible for ensuring that http clients are created with
// the appropriate configuration.
//
// This is the first iteration of the http client worker, and it is expected
// that each http client is correctly tracked and managed by the worker. If
// proxy settings change, the worker will update the http client accordingly.
package httpclient
