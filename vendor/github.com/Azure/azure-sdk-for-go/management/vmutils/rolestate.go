package vmutils

import (
	"time"

	"github.com/Azure/azure-sdk-for-go/management"
	vm "github.com/Azure/azure-sdk-for-go/management/virtualmachine"
)

// WaitForDeploymentPowerState blocks until all role instances in deployment to
// reach desired power state.
func WaitForDeploymentPowerState(client management.Client, cloudServiceName, deploymentName string, desiredPowerstate vm.PowerState) error {
	for {
		deployment, err := vm.NewClient(client).GetDeployment(cloudServiceName, deploymentName)
		if err != nil {
			return err
		}
		if allInstancesInPowerState(deployment.RoleInstanceList, desiredPowerstate) {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
}

func allInstancesInPowerState(instances []vm.RoleInstance, desiredPowerstate vm.PowerState) bool {
	for _, r := range instances {
		if r.PowerState != desiredPowerstate {
			return false
		}
	}

	return true
}
