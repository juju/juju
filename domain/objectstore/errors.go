// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import "github.com/juju/errors"

// ErrHashAndSizeAlreadyExists is returned when a hash already exists, but
// the associated size is different. This should never happen, it means that
// there is a collision in the hash function.
const ErrHashAndSizeAlreadyExists = errors.ConstError("hash and size already exists")
