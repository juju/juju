// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package logging holds logging configuration for the controller. This
// includes the Loki push API endpoint configuration that is provided by the
// controller charm via the loki_push_api integration. The endpoint is
// persisted in the controller database and distributed to agents so they
// can send logs directly to Loki rather than through jujud-controller.
package logging
