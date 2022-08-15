// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cloudconfig/podcfg"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/docker"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretools "github.com/juju/juju/tools"
)

var errUpToDate = errors.AlreadyExistsf("no upgrades available")

func (m *ModelUpgraderAPI) decideVersion(
	targetVersion, agentVersion version.Number, agentStream string, st State, model Model,
) (_ version.Number, err error) {
	logger.Debugf("deciding target version for model upgrade, %q, %q, %q", targetVersion, agentVersion, agentStream)
	filterMajor := agentVersion.Major
	if targetVersion != version.Zero {
		filterMajor = targetVersion.Major
	}
	streamVersions, err := m.findTools(
		st, model, filterMajor, -1, agentVersion, "", "", agentStream,
	)
	if err != nil {
		return version.Zero, errors.Trace(err)
	}
	if targetVersion != version.Zero {
		if targetVersion.Compare(agentVersion.ToPatch()) < 0 {
			return version.Zero, errUpToDate
		}

		// If not completely specified already, pick a single tools version.
		filter := coretools.Filter{Number: targetVersion}
		packagedAgents, err := streamVersions.Match(filter)
		if err != nil {
			return version.Zero, errors.Wrap(err, errors.NotFoundf("no matching agent versions available"))
		}
		targetVersion, packagedAgents = packagedAgents.Newest()
		logger.Debugf("target version %q is the best version, packagedAgents %s", targetVersion, packagedAgents)
		return targetVersion, nil
	}

	// No explicitly specified version, so find the version to which we
	// need to upgrade. We find next available stable release to upgrade
	// to by incrementing the minor version, starting from the current
	// agent version and doing major.minor+1.patch=0.

	// Upgrading across a major release boundary requires that the version
	// be specified with --agent-version.
	nextVersion := agentVersion
	nextVersion.Minor += 1
	nextVersion.Patch = 0
	// Set Tag to space so it will be considered lexicographically earlier
	// than any tagged version.
	nextVersion.Tag = " "

	newestNextStable, found := streamVersions.NewestCompatible(nextVersion)
	if found {
		logger.Debugf("found a more recent stable version %s", newestNextStable)
		targetVersion = newestNextStable
		return targetVersion, nil
	}
	newestCurrent, found := streamVersions.NewestCompatible(agentVersion)
	if found {
		if newestCurrent.Compare(agentVersion) == 0 {
			return version.Zero, errUpToDate
		}
		if newestCurrent.Compare(agentVersion) > 0 {
			targetVersion = newestCurrent
			logger.Debugf("found more recent current version %s", newestCurrent)
			return targetVersion, nil
		}
	}

	// no available tool found, CLI could upload the local build and it's allowed.
	return version.Zero, errors.NewNotFound(nil, "available agent tool, upload required")
}

func (m *ModelUpgraderAPI) findTools(
	st State, model Model,
	majorVersion, minorVersion int, agentVersion version.Number, osType, arch, agentStream string,
) (coretools.Versions, error) {
	result, err := m.toolsFinder.FindTools(params.FindToolsParams{
		MajorVersion: majorVersion,
		MinorVersion: minorVersion,
		Arch:         arch,
		OSType:       osType,
		AgentStream:  agentStream,
	})
	if err == nil && result.Error != nil {
		err = apiservererrors.RestoreError(result.Error)
	}
	if model.Type() != state.ModelTypeCAAS {
		// We return now for non CAAS model.
		return toolListToVersions(result.List), errors.Annotate(err, "cannot find tool version from simple streams")
	}
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	streamsVersions := set.NewStrings()
	for _, a := range result.List {
		streamsVersions.Add(a.Version.Number.String())
	}
	logger.Tracef("versions from simplestream %v", streamsVersions.SortedValues())
	return m.toolVersionsForCAAS(
		st, model, majorVersion, minorVersion, agentVersion, osType, arch, agentStream, streamsVersions,
	)
}

// The default available agents come directly from streams metadata.
func toolListToVersions(streamsVersions coretools.List) coretools.Versions {
	agents := make(coretools.Versions, len(streamsVersions))
	for i, t := range streamsVersions {
		agents[i] = t
	}
	return agents
}

func (m *ModelUpgraderAPI) toolVersionsForCAAS(
	st State, model Model,
	majorVersion, minorVersion int, agentVersion version.Number, osType, archFilter, agentStream string,
	streamsVersions set.Strings,
) (coretools.Versions, error) {
	result := coretools.Versions{}
	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	imageRepoDetails := controllerCfg.CAASImageRepo()
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
	imageName := podcfg.JujudOCIName
	tags, err := reg.Tags(imageName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, tag := range tags {
		number := tag.AgentVersion()
		if number.Compare(agentVersion) <= 0 {
			continue
		}
		if agentVersion.Build == 0 && number.Build > 0 {
			continue
		}
		if majorVersion != -1 && number.Major != majorVersion {
			continue
		}
		if !controllerCfg.Features().Contains(feature.DeveloperMode) && streamsVersions.Size() > 0 {
			numberCopy := number
			numberCopy.Build = 0
			if !streamsVersions.Contains(numberCopy.String()) {
				continue
			}
		} else {
			// Fallback for when we can't query the streams versions.
			// Ignore tagged (non-release) versions if agent stream is released.
			if (agentStream == "" || agentStream == envtools.ReleasedStream) && number.Tag != "" {
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
		if archFilter != "" && arch != archFilter {
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
