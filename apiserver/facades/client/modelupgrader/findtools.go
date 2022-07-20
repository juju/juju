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

func isClientPublished(clientVersion version.Number, streamVersions coretools.Versions) bool {
	for _, v := range streamVersions {
		if v.AgentVersion().Compare(clientVersion) == 0 {
			return true
		}
	}
	return false
}

func checkClientCompatibility(clientVersion, targetVersion, agentVersion version.Number) (bool, error) {
	switch clientVersion.Major - agentVersion.Major {
	case 0:
		return false, nil
	case 1:
		// The running model is the previous major version.
		if targetVersion == version.Zero || targetVersion.Major == agentVersion.Major {
			// Not requesting an upgrade across major release boundary.
			// Warn of incompatible CLI and filter on the prior major version
			// when searching for available tools.
			// TODO(cherylj) Add in a suggestion to upgrade to 2.0 if
			// no matching tools are found (bug 1532670)
			return true, nil
		}
		return false, nil
	default:
		// This version of juju client cannot upgrade the running
		// model version (can't guarantee API compatibility).
		return false, errors.Errorf("cannot upgrade a %s model with a %s client", agentVersion, clientVersion)
	}
}

func (m *ModelUpgraderAPI) decideVersion(
	clientVersion, targetVersion, agentVersion version.Number, officialClient bool,
	agentStream string, st State, model Model,
) (_ version.Number, _ bool, err error) {
	logger.Criticalf(
		"decideVersion => clientVersion %q, targetVersion %q, agentVersion  %q, officialClient %v, agentStream %q",
		clientVersion, targetVersion, agentVersion, officialClient, agentStream,
	)
	defer func() {
		logger.Criticalf("decideVersion err %#v", err)
	}()
	filterOnPrior, err := checkClientCompatibility(clientVersion, targetVersion, agentVersion)
	if err != nil {
		return version.Zero, false, errors.Trace(err)
	}
	filterVersion := clientVersion
	if targetVersion != version.Zero {
		filterVersion = targetVersion
	} else if filterOnPrior {
		filterVersion.Major--
	}
	streamVersions, err := m.findTools(
		st, model, filterVersion.Major, -1, agentVersion, "", "", agentStream,
	)
	if err != nil {
		return version.Zero, false, errors.Trace(err)
	}
	// TODO: refactor upgradeCtx.maybeChoosePackagedAgent
	// if targetVersion == version.Zero &&  streamVersions.newestNextStable found or streamVersions.newestCurrent found {
	// 	return newestNextStable or newestCurrent, nil
	// }
	if targetVersion != version.Zero {
		// If not completely specified already, pick a single tools version.
		filter := coretools.Filter{Number: targetVersion}
		packagedAgents, err := streamVersions.Match(filter)
		if err != nil {
			return version.Zero, false, errors.Wrap(err, errors.New("no matching agent versions available"))
		}
		targetVersion, packagedAgents = packagedAgents.Newest()
		logger.Criticalf("found targetVersion %q, packagedAgents %#v", targetVersion, packagedAgents)
		return targetVersion, false, nil
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
		// logger.Debugf("found a more recent stable version %s", newestNextStable)
		logger.Criticalf("found a more recent stable version %s", newestNextStable)
		targetVersion = newestNextStable
		return targetVersion, false, nil
	}
	newestCurrent, found := streamVersions.NewestCompatible(agentVersion)
	if found {
		if newestCurrent.Compare(agentVersion) == 0 {
			return version.Zero, false, errUpToDate(agentVersion) // TODO !!!!
		}
		if newestCurrent.Compare(agentVersion) > 0 {
			targetVersion = newestCurrent
			// logger.Debugf("found more recent current version %s", newestCurrent)
			logger.Criticalf("found more recent current version %s", newestCurrent)
			return targetVersion, false, nil
		}
	}
	canImplicitUpload := checkCanImplicitUpload(
		model, clientVersion, agentVersion, officialClient,
		isClientPublished(clientVersion, streamVersions),
	)
	logger.Criticalf("checkCanImplicitUpload canImplicitUpload %v", canImplicitUpload)

	logger.Criticalf("fetched stream versions %s, canImplicitUpload %v", streamVersions, canImplicitUpload)
	if canImplicitUpload {
		return version.Zero, true, errors.NewNotFound(nil, "available agent tool, upload required")
	}
	// no available tool found, and we are not allowed to upload.
	return version.Zero, false, errors.New("no more recent supported versions available")
}

func checkCanImplicitUpload(
	model Model, clientVersion, agentVersion version.Number,
	isOfficialClient, isClientPublished bool,
) bool {
	logger.Criticalf(
		"checkCanImplicitUpload clientVersion %q, agentVersion %q, isOfficialClient %v, isClientPublished %v",
		clientVersion, agentVersion, isOfficialClient, isClientPublished,
	)
	if model.Type() != state.ModelTypeIAAS {
		return false
	}
	newerClient := clientVersion.Compare(agentVersion) > 0
	if !newerClient {
		return false
	}
	if !isOfficialClient {
		// For non official (under $GOPATH) client, always use --build-agent explicitly.
		return false
	}
	if isClientPublished {
		// For official (under /snap/juju/bin) client, upload only if the client is not a published version.
		return false
	}
	if agentVersion.Build == 0 && clientVersion.Build == 0 {
		return false
	}
	return true
}

func errUpToDate(agentVersion version.Number) error {
	// TODO !!!!
	return errors.AlreadyExistsf("errUpToDate %q", agentVersion)
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
		logger.Criticalf("%q.Compare(%q) %v", number, agentVersion, number.Compare(agentVersion))
		if number.Compare(agentVersion) <= 0 {
			continue
		}
		if agentVersion.Build == 0 && number.Build > 0 {
			continue
		}
		logger.Criticalf("number.Major %v, majorVersion %v", number.Major, majorVersion)
		if majorVersion != -1 && number.Major != majorVersion {
			continue
		}
		if !controllerCfg.Features().Contains(feature.DeveloperMode) && streamsVersions.Size() > 0 {
			numberCopy := number
			numberCopy.Build = 0
			logger.Criticalf("streamsVersions.Contains(%q) %v", numberCopy, streamsVersions.Contains(numberCopy.String()))
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
		logger.Criticalf("arch %q, archFilter %q", arch, archFilter)
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
		logger.Criticalf("tools %q", tools)
		result = append(result, &tools)
	}
	return result, nil
}
