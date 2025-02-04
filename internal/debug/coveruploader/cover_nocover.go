// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !cover

package coveruploader

// Enable is a no-op without the cover build tag.
func Enable() {}
