// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/http/v2"
	"github.com/juju/loggo"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"

	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/keys"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.environs.sync")

// SyncContext describes the context for tool synchronization.
type SyncContext struct {
	// TargetToolsFinder is a ToolsFinder provided to find existing
	// tools in the target destination.
	TargetToolsFinder ToolsFinder

	// TargetToolsUploader is a ToolsUploader provided to upload
	// tools to the target destination.
	TargetToolsUploader ToolsUploader

	// AllVersions controls the copy of all versions, not only the latest.
	AllVersions bool

	// Copy tools with major version, if MajorVersion > 0.
	MajorVersion int

	// Copy tools with minor version, if MinorVersion > 0.
	MinorVersion int

	// DryRun controls that nothing is copied. Instead it's logged
	// what would be coppied.
	DryRun bool

	// Stream specifies the simplestreams stream to use (defaults to "Released").
	Stream string

	// Source, if non-empty, specifies a directory in the local file system
	// to use as a source.
	Source string
}

// ToolsFinder provides an interface for finding tools of a specified version.
type ToolsFinder interface {
	// FindTools returns a list of tools with the specified major version in the specified stream.
	FindTools(major int, stream string) (coretools.List, error)
}

// ToolsUploader provides an interface for uploading tools and associated
// metadata.
type ToolsUploader interface {
	// UploadTools uploads the tools with the specified version and tarball contents.
	UploadTools(toolsDir, stream string, tools *coretools.Tools, data []byte) error
}

// SyncTools copies the Juju tools tarball from the official bucket
// or a specified source directory into the user's environment.
func SyncTools(syncContext *SyncContext) error {
	sourceDataSource, err := selectSourceDatasource(syncContext)
	if err != nil {
		return errors.Trace(err)
	}

	logger.Infof("listing available agent binaries")
	if syncContext.MajorVersion == 0 && syncContext.MinorVersion == 0 {
		syncContext.MajorVersion = jujuversion.Current.Major
		syncContext.MinorVersion = -1
		if !syncContext.AllVersions {
			syncContext.MinorVersion = jujuversion.Current.Minor
		}
	}

	toolsDir := syncContext.Stream
	// If no stream has been specified, assume "released" for non-devel versions of Juju.
	if syncContext.Stream == "" {
		// We now store the tools in a directory named after their stream, but the
		// legacy behaviour is to store all tools in a single "releases" directory.
		toolsDir = envtools.ReleasedStream
		// Always use the primary stream here - the user can specify
		// to override that decision.
		syncContext.Stream = envtools.PreferredStreams(&jujuversion.Current, false, "")[0]
	}
	// TODO (stickupkid): We should lift this simplestreams constructor out of
	// this function.
	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	// For backwards compatibility with cloud storage, if there are no tools in the specified stream,
	// double check the release stream.
	// TODO - remove this when we no longer need to support cloud storage upgrades.
	streams := []string{syncContext.Stream, envtools.ReleasedStream}
	sourceTools, err := envtools.FindToolsForCloud(
		ss,
		[]simplestreams.DataSource{sourceDataSource}, simplestreams.CloudSpec{},
		streams, syncContext.MajorVersion, syncContext.MinorVersion, coretools.Filter{})
	if err != nil {
		return errors.Trace(err)
	}

	logger.Infof("found %d agent binaries", len(sourceTools))
	if !syncContext.AllVersions {
		var latest version.Number
		latest, sourceTools = sourceTools.Newest()
		logger.Infof("found %d recent agent binaries (version %s)", len(sourceTools), latest)
	}
	for _, tool := range sourceTools {
		logger.Debugf("found source agent binary: %v", tool)
	}

	logger.Infof("listing target agent binaries storage")
	targetTools, err := syncContext.TargetToolsFinder.FindTools(syncContext.MajorVersion, syncContext.Stream)
	switch err {
	case nil, coretools.ErrNoMatches, envtools.ErrNoTools:
	default:
		return errors.Trace(err)
	}
	for _, tool := range targetTools {
		logger.Debugf("found target agent binary: %v", tool)
	}

	missing := sourceTools.Exclude(targetTools)
	logger.Infof("found %d agent binaries in target; %d agent binaries to be copied", len(targetTools), len(missing))
	if syncContext.DryRun {
		for _, tools := range missing {
			logger.Infof("copying %s from %s", tools.Version, tools.URL)
		}
		return nil
	}

	err = copyTools(toolsDir, syncContext.Stream, missing, syncContext.TargetToolsUploader)
	if err != nil {
		return err
	}
	logger.Infof("copied %d agent binaries", len(missing))
	return nil
}

// selectSourceDatasource returns a storage reader based on the source setting.
func selectSourceDatasource(syncContext *SyncContext) (simplestreams.DataSource, error) {
	source := syncContext.Source
	if source == "" {
		source = envtools.DefaultBaseURL
	}
	sourceURL, err := envtools.ToolsURL(source)
	if err != nil {
		return nil, err
	}
	logger.Infof("source for sync of agent binaries: %v", sourceURL)
	config := simplestreams.Config{
		Description:          "sync agent binaries source",
		BaseURL:              sourceURL,
		PublicSigningKey:     keys.JujuPublicKey,
		HostnameVerification: true,
		Priority:             simplestreams.CUSTOM_CLOUD_DATA,
	}
	if err := config.Validate(); err != nil {
		return nil, errors.Annotate(err, "simplestreams config validation failed")
	}
	return simplestreams.NewDataSource(config), nil
}

// copyTools copies a set of tools from the source to the target.
func copyTools(toolsDir, stream string, tools []*coretools.Tools, u ToolsUploader) error {
	for _, tool := range tools {
		logger.Infof("copying %s from %s", tool.Version, tool.URL)
		if err := copyOneToolsPackage(toolsDir, stream, tool, u); err != nil {
			return err
		}
	}
	return nil
}

// copyOneToolsPackage copies one tool from the source to the target.
func copyOneToolsPackage(toolsDir, stream string, tools *coretools.Tools, u ToolsUploader) error {
	toolsName := envtools.StorageName(tools.Version, toolsDir)
	logger.Infof("downloading %q %v (%v)", stream, toolsName, tools.URL)
	client := http.NewClient()
	resp, err := client.Get(context.TODO(), tools.URL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	// Verify SHA-256 hash.
	var buf bytes.Buffer
	sha256, size, err := utils.ReadSHA256(io.TeeReader(resp.Body, &buf))
	if err != nil {
		return err
	}
	if tools.SHA256 == "" {
		logger.Errorf("no SHA-256 hash for %v", tools.SHA256) // TODO(dfc) can you spot the bug ?
	} else if sha256 != tools.SHA256 {
		return errors.Errorf("SHA-256 hash mismatch (%v/%v)", sha256, tools.SHA256)
	}
	sizeInKB := (size + 512) / 1024
	logger.Infof("uploading %v (%dkB) to model", toolsName, sizeInKB)
	return u.UploadTools(toolsDir, stream, tools, buf.Bytes())
}

// UploadFunc is the type of Upload, which may be
// reassigned to control the behaviour of tools
// uploading.
type UploadFunc func(envtools.SimplestreamsFetcher, storage.Storage, string, *version.Number) (*coretools.Tools, error)

// Upload is exported for testing.
var Upload UploadFunc = upload

// upload builds whatever version of github.com/juju/juju is in $GOPATH,
// uploads it to the given storage, and returns a Tools instance describing
// them. If forceVersion is not nil, the uploaded tools bundle will report
// the given version number.
func upload(ss envtools.SimplestreamsFetcher, store storage.Storage, stream string, forceVersion *version.Number) (*coretools.Tools, error) {
	builtTools, err := BuildAgentTarball(true, forceVersion, stream)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(builtTools.Dir)
	return syncBuiltTools(ss, store, stream, builtTools)
}

// generateAgentMetadata copies the built tools tarball into a tarball for the specified
// stream and series and generates corresponding metadata.
func generateAgentMetadata(ss envtools.SimplestreamsFetcher, toolsInfo *BuiltAgent, stream string) error {
	// Copy the tools to the target storage, recording a Tools struct for each one.
	var targetTools coretools.List
	targetTools = append(targetTools, &coretools.Tools{
		Version: toolsInfo.Version,
		Size:    toolsInfo.Size,
		SHA256:  toolsInfo.Sha256Hash,
	})
	// The tools have been copied to a temp location from which they will be uploaded,
	// now write out the matching simplestreams metadata so that SyncTools can find them.
	metadataStore, err := filestorage.NewFileStorageWriter(toolsInfo.Dir)
	if err != nil {
		return err
	}
	logger.Debugf("generating agent metadata")
	return envtools.MergeAndWriteMetadata(ss, metadataStore, stream, stream, targetTools, false)
}

// BuiltAgent contains metadata for a tools tarball resulting from
// a call to BundleTools.
type BuiltAgent struct {
	Version     version.Binary
	Official    bool
	Dir         string
	StorageName string
	Sha256Hash  string
	Size        int64
}

// BuildAgentTarballFunc is a function which can build an agent tarball.
type BuildAgentTarballFunc func(build bool, forceVersion *version.Number, stream string) (*BuiltAgent, error)

// Override for testing.
var BuildAgentTarball BuildAgentTarballFunc = buildAgentTarball

// BuildAgentTarball bundles an agent tarball and places it in a temp directory in
// the expected agent path.
func buildAgentTarball(build bool, forceVersion *version.Number, stream string) (_ *BuiltAgent, err error) {
	// TODO(rog) find binaries from $PATH when not using a development
	// version of juju within a $GOPATH.

	logger.Debugf("Making agent binary tarball")
	// We create the entire archive before asking the environment to
	// start uploading so that we can be sure we have archived
	// correctly.
	f, err := ioutil.TempFile("", "juju-tgz")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer os.Remove(f.Name())
	toolsVersion, official, sha256Hash, err := envtools.BundleTools(build, f, forceVersion)
	if err != nil {
		return nil, err
	}
	// Built agent version needs to match the client used to bootstrap.
	builtVersion := toolsVersion
	builtVersion.Build = 0
	clientVersion := jujuversion.Current
	clientVersion.Build = 0
	if builtVersion.Number.Compare(clientVersion) != 0 {
		return nil, errors.Errorf("agent binary %v not compatible with bootstrap client %v", toolsVersion.Number, jujuversion.Current)
	}
	fileInfo, err := f.Stat()
	if err != nil {
		return nil, errors.Errorf("cannot stat newly made agent binary archive: %v", err)
	}
	size := fileInfo.Size()
	reportedVersion := toolsVersion
	if !official && forceVersion != nil {
		reportedVersion.Number = *forceVersion
	}
	if official {
		logger.Infof("using official agent binary %v (%dkB)", toolsVersion, (size+512)/1024)
	} else {
		logger.Infof("using agent binary %v aliased to %v (%dkB)", toolsVersion, reportedVersion, (size+512)/1024)
	}
	baseToolsDir, err := ioutil.TempDir("", "juju-tools")
	if err != nil {
		return nil, err
	}

	// If we exit with an error, clean up the built tools directory.
	defer func() {
		if err != nil {
			os.RemoveAll(baseToolsDir)
		}
	}()

	err = os.MkdirAll(filepath.Join(baseToolsDir, storage.BaseToolsPath, stream), 0755)
	if err != nil {
		return nil, err
	}
	storageName := envtools.StorageName(toolsVersion, stream)
	err = utils.CopyFile(filepath.Join(baseToolsDir, storageName), f.Name())
	if err != nil {
		return nil, err
	}
	return &BuiltAgent{
		Version:     toolsVersion,
		Official:    official,
		Dir:         baseToolsDir,
		StorageName: storageName,
		Size:        size,
		Sha256Hash:  sha256Hash,
	}, nil
}

// syncBuiltTools copies to storage a tools tarball and cloned copies for each series.
func syncBuiltTools(ss envtools.SimplestreamsFetcher, store storage.Storage, stream string, builtTools *BuiltAgent) (*coretools.Tools, error) {
	if err := generateAgentMetadata(ss, builtTools, stream); err != nil {
		return nil, err
	}
	syncContext := &SyncContext{
		Source:            builtTools.Dir,
		TargetToolsFinder: StorageToolsFinder{store},
		TargetToolsUploader: StorageToolsUploader{
			Fetcher:       ss,
			Storage:       store,
			WriteMetadata: false,
			WriteMirrors:  false,
		},
		AllVersions:  true,
		Stream:       stream,
		MajorVersion: builtTools.Version.Major,
		MinorVersion: -1,
	}
	logger.Debugf("uploading agent binaries to cloud storage")
	err := SyncTools(syncContext)
	if err != nil {
		return nil, err
	}
	url, err := store.URL(builtTools.StorageName)
	if err != nil {
		return nil, err
	}
	return &coretools.Tools{
		Version: builtTools.Version,
		URL:     url,
		Size:    builtTools.Size,
		SHA256:  builtTools.Sha256Hash,
	}, nil
}

// StorageToolsFinder is an implementation of ToolsFinder
// that searches for tools in the specified storage.
type StorageToolsFinder struct {
	Storage storage.StorageReader
}

func (f StorageToolsFinder) FindTools(major int, stream string) (coretools.List, error) {
	return envtools.ReadList(f.Storage, stream, major, -1)
}

// StorageToolsUploader is an implementation of ToolsUploader that
// writes tools to the provided storage and then writes merged
// metadata, optionally with mirrors.
type StorageToolsUploader struct {
	Fetcher       envtools.SimplestreamsFetcher
	Storage       storage.Storage
	WriteMetadata bool
	WriteMirrors  envtools.ShouldWriteMirrors
}

func (u StorageToolsUploader) UploadTools(toolsDir, stream string, tools *coretools.Tools, data []byte) error {
	toolsName := envtools.StorageName(tools.Version, toolsDir)
	if err := u.Storage.Put(toolsName, bytes.NewReader(data), int64(len(data))); err != nil {
		return err
	}
	if !u.WriteMetadata {
		return nil
	}
	err := envtools.MergeAndWriteMetadata(u.Fetcher, u.Storage, toolsDir, stream, coretools.List{tools}, u.WriteMirrors)
	if err != nil {
		logger.Errorf("error writing agent metadata: %v", err)
		return err
	}
	return nil
}
