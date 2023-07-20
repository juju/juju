// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cloudconfig/podcfg"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/docker"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	coretools "github.com/juju/juju/tools"
)

var errUpToDate = errors.AlreadyExistsf("no upgrades available")

func (m *ModelUpgraderAPI) decideVersion(
	currentVersion version.Number, args common.FindAgentsParams,
) (_ version.Number, err error) {

	// Short circuit expensive agent look up if we are already up-to-date.
	if args.Number != version.Zero && args.Number.Compare(currentVersion.ToPatch()) <= 0 {
		return version.Zero, errUpToDate
	}

	streamVersions, err := m.findAgents(args)
	if err != nil {
		return version.Zero, errors.Trace(err)
	}
	if args.Number != version.Zero {
		// Not completely specified already, so pick a single agent version.
		filter := coretools.Filter{Number: args.Number}
		packagedAgents, err := streamVersions.Match(filter)
		if err != nil {
			return version.Zero, errors.Wrap(err, errors.NotFoundf("no matching agent versions available"))
		}
		var targetVersion version.Number
		targetVersion, packagedAgents = packagedAgents.Newest()
		m.logger.Debugf("target version %q is the best version, packagedAgents %v", targetVersion, packagedAgents)
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
			return version.Zero, errUpToDate
		}
		if newestCurrent.Compare(currentVersion) > 0 {
			m.logger.Debugf("found more recent agent version %s", newestCurrent)
			return newestCurrent, nil
		}
	}

	// no available tool found, CLI could upload the local build and it's allowed.
	return version.Zero, errors.NewNotFound(nil, "available agent binary, upload required")
}

func (m *ModelUpgraderAPI) findAgents(
	args common.FindAgentsParams,
) (coretools.Versions, error) {
	list, err := m.toolsFinder.FindAgents(args, m.ctrlConfigService)
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
	imageRepoDetails := args.ControllerCfg.CAASImageRepo()
	if imageRepoDetails.Empty() {
		repoDetails, err := docker.NewImageRepoDetails(podcfg.JujudOCINamespace)
		if err != nil {
			return nil, errors.Trace(err)
		}
		imageRepoDetails = *repoDetails
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
	m.logger.Tracef("versions from simplestreams %v", streamsVersions)
	imageName := podcfg.JujudOCIName
	tags, err := reg.Tags(imageName)
	if err != nil {
		return nil, errors.Trace(err)
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
		if args.Number != version.Zero && args.Number.Compare(number) != 0 {
			continue
		}
		if !args.ControllerCfg.Features().Contains(feature.DeveloperMode) && streamsVersions.Size() > 0 {
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
		arch, err := reg.GetArchitecture(imageName, number.String())
		if errors.Is(err, errors.NotFound) {
			continue
		}
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get architecture for %s:%s", imageName, number.String())
		}
		if args.Arch != "" && arch != args.Arch {
			continue
		}
		tools := coretools.Tools{
			Version: version.Binary{
				Number:  number,
				Release: coreos.HostOSTypeName(),
				Arch:    arch,
			},
		}
		result = append(result, &tools)
	}
	return result, nil
}
