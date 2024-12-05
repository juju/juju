// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import "github.com/juju/juju/internal/errors"

const (
	// UnknownResourceType describes an error where the resource type is
	// not oci-image or file.
	UnknownResourceType = errors.ConstError("unknown resource type")
)
