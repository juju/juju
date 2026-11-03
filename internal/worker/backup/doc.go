// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package backup owns archive generation and its controller-wide lease.
// Its manifold outputs a creation function for the API server. Generation and
// renewal share cancellation; the lease is released before archive download.
package backup
