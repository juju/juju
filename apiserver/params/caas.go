// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/juju/core/constraints"
	"github.com/juju/version"
)

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

// CAASApplicationProvisioningInfo holds info needed to provision a caas application.
type CAASApplicationProvisioningInfo struct {
	ImagePath    string                       `json:"image-path"`
	Version      version.Number               `json:"version"`
	APIAddresses []string                     `json:"api-addresses"`
	CACert       string                       `json:"ca-cert"`
	Constraints  constraints.Value            `json:"constraints"`
	Tags         map[string]string            `json:"tags,omitempty"`
	Filesystems  []KubernetesFilesystemParams `json:"filesystems,omitempty"`
	Volumes      []KubernetesVolumeParams     `json:"volumes,omitempty"`
	Devices      []KubernetesDeviceParams     `json:"devices,omitempty"`
	Error        *Error                       `json:"error,omitempty"`
}

// CAASApplicationGarbageCollectArg holds info needed to cleanup units that have
// gone away permanently.
type CAASApplicationGarbageCollectArg struct {
	Application     Entity   `json:"application"`
	ObservedUnits   Entities `json:"observed-units"`
	DesiredReplicas int      `json:"desired-replicas"`
	ActivePodNames  []string `json:"active-pod-names"`
}

// CAASApplicationGarbageCollectArgs holds info needed to cleanup units that have
// gone away permanently.
type CAASApplicationGarbageCollectArgs struct {
	Args []CAASApplicationGarbageCollectArg `json:"args"`
}
