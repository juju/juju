// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package loki provides a worker for pushing log records to a
// Loki instance using the HTTP push API. The worker buffers
// records and flushes them based on batch size or a time
// interval, with retries and exponential backoff. Records are
// grouped by label set into Loki streams.
package loki
