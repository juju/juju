// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead pass one in to the NewResolver function.
type logger interface{}

var _ logger = struct{}{}
