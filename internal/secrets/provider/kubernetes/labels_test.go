// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"testing"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

// TestServiceAccountLabelsMatchAppCreatedSelector verifies that the labels
// applied to secrets provider resources (service accounts, roles, rolebindings)
// are a superset of the LabelsForAppCreated selector used by app.Delete() for
// label-based K8s resource cleanup. If this test fails, removed applications
// will leave orphaned secrets provider resources behind in the cluster.
func TestServiceAccountLabelsMatchAppCreatedSelector(t *testing.T) {
	const (
		appName        = "zinc-k8s"
		modelName      = "my-model"
		modelUUID      = "deadbeef-0bad-400d-8000-4b1d0d06f00d"
		controllerUUID = "deadbeef-1bad-500d-9000-4b1d0d06f00d"
	)

	// Build labels exactly as ensureSecretAccessToken does:
	// 1. Start with labelsForServiceAccount (base labels)
	// 2. Merge in app name labels including LabelJujuAppCreatedBy
	resourceLabels := labelsForServiceAccount(modelName, modelUUID)
	resourceLabels = utils.LabelsMerge(resourceLabels, map[string]string{
		constants.LabelKubernetesAppName: appName,
		constants.LabelJujuAppCreatedBy:  appName,
	})

	// Build the selector that app.Delete() uses to find resources to clean up.
	cleanupSelector := utils.LabelsToSelector(
		utils.LabelsForAppCreated(appName, modelName, modelUUID, controllerUUID, constants.LabelVersion2),
	)

	if !cleanupSelector.Matches(resourceLabels) {
		t.Errorf("secrets provider resource labels do not match app.Delete() cleanup selector\n"+
			"resource labels: %v\n"+
			"cleanup selector: %v",
			resourceLabels, cleanupSelector)
	}
}
