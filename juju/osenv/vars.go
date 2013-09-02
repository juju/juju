// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

const (
	JujuEnv        = "JUJU_ENV"
	JujuHome       = "JUJU_HOME"
	JujuRepository = "JUJU_REPOSITORY"
	// TODO(thumper): 2013-09-02 bug 1219630
	// As much as I'd like to remove JujuContainerType now, it is still
	// needed as MAAS still needs it at this stage, and we can't fix
	// everything at once.
	JujuContainerType = "JUJU_CONTAINER_TYPE"
)
