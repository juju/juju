// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package secretsrevoker provides a worker for revoking issued backend tokens
// and cleaning them up when they expire.
// NOTE: In 4.0 this could be removed and become a cleanup job.
package secretsrevoker
