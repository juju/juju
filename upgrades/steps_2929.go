// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2929 returns database upgrade steps for Juju 2.9.29
func stateStepsFor2929() []Step {
	return []Step{
		&upgradeStep{
			description: "add controller config for grpc-api-port and grpc-gateway-api-port",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddControllerConfigGrpcAPIPorts()
			},
		},
	}
}
