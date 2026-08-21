// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package objectstorefacade wraps the object store with fortress gating for
// write operations.
//
// Only writes are guarded by the fortress. When the object store drainer
// enters the Draining phase to migrate blobs from file-backed to S3 storage,
// it locks the fortress to block new writes. This prevents writes to the old
// file-backed store from being lost after the backend switch to S3.
//
// Reads (Get, GetBySHA256, GetBySHA256Prefix) bypass the fortress entirely.
// During draining, the old file-backed store is still running and serving
// data safely. Guarding reads turned a milliseconds-scale race into an
// hours-scale outage for callers like the uniter.
//
// # Invariants
//
//   - Writes are blocked during draining. No write to the old store can be
//     silently lost after the backend switch.
//
//   - Reads succeed during draining against the old store. Callers do not
//     need to retry until draining completes.
//
//   - A reader that acquired an io.ReadCloser before the FlushWorkers step
//     may observe a transient IO error. The fortress never eliminated this
//     race; it only made it harder to hit.
package objectstorefacade