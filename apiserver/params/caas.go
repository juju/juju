// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import "github.com/juju/version"

// CAASUnitIntroductionArgs is used by embedded units to introduce
// themselves via CAASApplication facade.
type CAASUnitIntroductionArgs struct {
	PodName string `json:"pod-name"`
	PodUUID string `json:"pod-uuid"`
}

// CAASUnitIntroduction contains the agent config for CAASApplication units.
type CAASUnitIntroduction struct {
	UnitName  string `json:"unit-name"`
	AgentConf []byte `json:"agent-conf"`
}

// CAASUnitIntroductionResult is returned from CAASApplication facade.
type CAASUnitIntroductionResult struct {
	Result *CAASUnitIntroduction `json:"result,omitempty"`
	Error  *Error                `json:"error,omitempty"`
}

// CAASApplicationProvisioningInfoResults holds OperatorProvisioningInfo results.
type CAASApplicationProvisioningInfoResults struct {
	Results []CAASApplicationProvisioningInfo `json:"results"`
}

// CAASApplicationProvisioningInfo holds info need to provision a caas application.
type CAASApplicationProvisioningInfo struct {
	ImagePath    string                      `json:"image-path"`
	Version      version.Number              `json:"version"`
	APIAddresses []string                    `json:"api-addresses"`
	CACert       string                      `json:"ca-cert"`
	Tags         map[string]string           `json:"tags,omitempty"`
	CharmStorage *KubernetesFilesystemParams `json:"charm-storage,omitempty"`
	Error        *Error                      `json:"error,omitempty"`
}
