// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/featureflag"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/state"
)

var errUpToDate = errors.AlreadyExistsf("no upgrades available")

func (m *ModelUpgraderAPI) decideVersion(
	ctx context.Context,
	currentVersion semversion.Number, args common.FindAgentsParams,
) (_ semversion.Number, err error) {

	// Short circuit expensive agent look up if we are already up-to-date.
	if args.Number != semversion.Zero && args.Number.Compare(currentVersion.ToPatch()) <= 0 {
		return semversion.Zero, errUpToDate
	}

	streamVersions, err := m.findAgents(ctx, args)
	if err != nil {
		return semversion.Zero, errors.Trace(err)
	}
	if args.Number != semversion.Zero {
		// Not completely specified already, so pick a single agent version.
		filter := coretools.Filter{Number: args.Number}
		packagedAgents, err := streamVersions.Match(filter)
		if err != nil {
			return semversion.Zero, errors.Wrap(err, errors.NotFoundf("no matching agent versions available"))
		}
		var targetVersion semversion.Number
		targetVersion, packagedAgents = packagedAgents.Newest()
		m.logger.Debugf(context.TODO(), "target version %q is the best version, packagedAgents %v", targetVersion, packagedAgents)
		return targetVersion, nil
	}

	// No explicitly specified version, so find the version to which we
	// need to upgrade. We take the current version in use and find the
	// highest minor version with the same major version number.
	// CAAS models exclude agents with dev builds unless the current version
	// is also a dev build.
	allowDevBuilds := args.ModelType == state.ModelTypeIAAS || currentVersion.Build > 0
	newestCurrent, found := streamVersions.NewestCompatible(currentVersion, allowDevBuilds)
	if found {
		if newestCurrent.Compare(currentVersion) == 0 {
			return semversion.Zero, errUpToDate
		}
		if newestCurrent.Compare(currentVersion) > 0 {
			m.logger.Debugf(context.TODO(), "found more recent agent version %s", newestCurrent)
			return newestCurrent, nil
		}
	}

	// no available tool found, CLI could upload the local build and it's allowed.
	return semversion.Zero, errors.NewNotFound(nil, "available agent binary, upload required")
}

func (m *ModelUpgraderAPI) findAgents(
	ctx context.Context,
	args common.FindAgentsParams,
) (coretools.Versions, error) {
	list, err := m.toolsFinder.FindAgents(ctx, args)
	if args.ModelType != state.ModelTypeCAAS {
		// We return now for non CAAS model.
		return toolListToVersions(list), errors.Annotate(err, "cannot find agents from simple streams")
	}
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	return m.agentVersionsForCAAS(args, list)
}

// The default available agents come directly from streams metadata.
func toolListToVersions(streamsVersions coretools.List) coretools.Versions {
	agents := make(coretools.Versions, len(streamsVersions))
	for i, t := range streamsVersions {
		agents[i] = t
	}
	return agents
}

func (m *ModelUpgraderAPI) agentVersionsForCAAS(
	args common.FindAgentsParams,
	streamsAgents coretools.List,
) (coretools.Versions, error) {
	result := coretools.Versions{}
	imageRepoDetails, err := docker.NewImageRepoDetails(args.ControllerCfg.CAASImageRepo())
	if err != nil {
		return nil, errors.Annotatef(err, "parsing %s", controller.CAASImageRepo)
	}
	if imageRepoDetails.Empty() {
		imageRepoDetails, err = docker.NewImageRepoDetails(podcfg.JujudOCINamespace)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	reg, err := m.registryAPIFunc(imageRepoDetails)
	if err != nil {
		return nil, errors.Annotatef(err, "constructing registry API for %s", imageRepoDetails)
	}
	defer func() { _ = reg.Close() }()
	streamsVersions := set.NewStrings()
	for _, a := range streamsAgents {
		streamsVersions.Add(a.Version.Number.String())
	}
	m.logger.Tracef(context.TODO(), "versions from simplestreams %v", streamsVersions)
	imageName := podcfg.JujudOCIName
	tags, err := reg.Tags(imageName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	wantArch := args.Arch
	if wantArch == "" {
		wantArch = arch.DefaultArchitecture
	}
	for _, tag := range tags {
		number := tag.AgentVersion()
		if args.MajorVersion > 0 {
			if number.Major != args.MajorVersion {
				continue
			}
			if args.MinorVersion >= 0 && number.Minor != args.MinorVersion {
				continue
			}
		}
		if args.Number != semversion.Zero && args.Number.Compare(number) != 0 {
			continue
		}
		if !args.ControllerCfg.Features().Contains(featureflag.DeveloperMode) && streamsVersions.Size() > 0 {
			if !streamsVersions.Contains(number.ToPatch().String()) {
				continue
			}
		} else {
			// Fallback for when we can't query the streams versions.
			// Ignore tagged (non-release) versions if agent stream is released.
			if (args.AgentStream == "" || args.AgentStream == envtools.ReleasedStream) && number.Tag != "" {
				continue
			}
		}
		arches, err := reg.GetArchitectures(imageName, number.String())
		if errors.Is(err, errors.NotFound) {
			continue
		}
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get architecture for %s:%s", imageName, number.String())
		}
		if !set.NewStrings(arches...).Contains(wantArch) {
			continue
		}
		tools := coretools.Tools{
			Version: semversion.Binary{
				Number:  number,
				Release: coreos.HostOSTypeName(),
				Arch:    wantArch,
			},
		}
		result = append(result, &tools)
	}
	return result, nil
}
