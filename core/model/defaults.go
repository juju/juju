// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import "github.com/juju/juju/core/user"

const (
	// ControllerModelName is the name given to the model that hosts the Juju
	// controller. This is a static value that we use for every Juju deployment.
	// It provides a common reference point that we can leverage in business
	// logic to ask questions and calculate defaults in Juju.
	ControllerModelName = "controller"

	// ControllerModelOwnerUsername is the user name of the owner that is
	// assigned to the controller model. This is a static value that we use for
	// every Juju deployment.
	ControllerModelOwnerUsername = user.AdminUserName
)
