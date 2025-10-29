package service

import (
	"context"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
)

type ControllerAndModelCacheAgentFinder struct {
	ctrlSt  AgentFinderControllerState
	modelSt AgentFinderControllerModelState
}

func NewControllerAndModelCacheAgentFinder(
	ctrSt AgentFinderControllerState,
	modelSt AgentFinderControllerModelState,
) *ControllerAndModelCacheAgentFinder {
	return &ControllerAndModelCacheAgentFinder{
		ctrlSt:  ctrSt,
		modelSt: modelSt,
	}
}

func (c ControllerAndModelCacheAgentFinder) Name() string {
	return "ControllerAndModelCacheAgentFinder"
}

func (c ControllerAndModelCacheAgentFinder) SearchForAgentVersions(
	ctx context.Context,
	version semversion.Number,
	stream *agentbinary.Stream,
	_ coretools.Filter,
) ([]semversion.Number, error) {
	var allVersions []semversion.Number
	// Find the agents in the controller cache.
	agentVersionsInController, err := c.ctrlSt.
		GetAgentVersionsWithStream(ctx, stream)
	if err != nil {
		return []semversion.Number{}, errors.Capture(err)
	}
	allVersions = append(allVersions, agentVersionsInController...)

	// Find the agents in the model cache.
	agentVersionsInModel, err := c.modelSt.
		GetAgentVersionsWithStream(ctx, stream)
	if err != nil {
		return []semversion.Number{}, errors.Capture(err)
	}
	allVersions = append(allVersions, agentVersionsInModel...)

	// Exclude the retrieved versions that don't match the major and minor number
	// of the supplied version.
	var versions []semversion.Number
	for _, v := range allVersions {
		if v.Major != version.Major {
			continue
		}
		if v.Minor != version.Minor {
			continue
		}
		versions = append(versions, version)
	}

	// Return the versions in the same major and minor number.
	return versions, nil
}
