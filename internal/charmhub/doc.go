// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package charmhub is an HTTP client for sending requests to the Charmhub API.
//
// Call NewClient to create a client, and then Client's methods to perform
// individual requests, such as "info" or "refresh".
//
// This package automatically handles retries, request logging, and so on.
// To enable fine-grained request logging, set the logging label "charmhub" to
// TRACE (or set "metrics" to TRACE for logging request times).
package charmhub
