// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	envtools "github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
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
		return err
	}

	logger.Infof("listing available tools")
	if syncContext.MajorVersion == 0 && syncContext.MinorVersion == 0 {
		syncContext.MajorVersion = version.Current.Major
		syncContext.MinorVersion = -1
		if !syncContext.AllVersions {
			syncContext.MinorVersion = version.Current.Minor
		}
	}

	toolsDir := syncContext.Stream
	// If no stream has been specified, assume "released" for non-devel versions of Juju.
	if syncContext.Stream == "" {
		// We now store the tools in a directory named after their stream, but the
		// legacy behaviour is to store all tools in a single "releases" directory.
		toolsDir = envtools.LegacyReleaseDirectory
		syncContext.Stream = envtools.PreferredStream(&version.Current.Number, false, syncContext.Stream)
	}
	sourceTools, err := envtools.FindToolsForCloud(
		[]simplestreams.DataSource{sourceDataSource}, simplestreams.CloudSpec{},
		syncContext.Stream, syncContext.MajorVersion, syncContext.MinorVersion, coretools.Filter{})
	// For backwards compatibility with cloud storage, if there are no tools in the specified stream,
	// double check the release stream.
	// TODO - remove this when we no longer need to support cloud storage upgrades.
	if err == envtools.ErrNoTools {
		sourceTools, err = envtools.FindToolsForCloud(
			[]simplestreams.DataSource{sourceDataSource}, simplestreams.CloudSpec{},
			envtools.ReleasedStream, syncContext.MajorVersion, syncContext.MinorVersion, coretools.Filter{})
	}
	if err != nil {
		return err
	}

	logger.Infof("found %d tools", len(sourceTools))
	if !syncContext.AllVersions {
		var latest version.Number
		latest, sourceTools = sourceTools.Newest()
		logger.Infof("found %d recent tools (version %s)", len(sourceTools), latest)
	}
	for _, tool := range sourceTools {
		logger.Debugf("found source tool: %v", tool)
	}

	logger.Infof("listing target tools storage")
	targetTools, err := syncContext.TargetToolsFinder.FindTools(syncContext.MajorVersion, syncContext.Stream)
	switch err {
	case nil, coretools.ErrNoMatches, envtools.ErrNoTools:
	default:
		return err
	}
	for _, tool := range targetTools {
		logger.Debugf("found target tool: %v", tool)
	}

	missing := sourceTools.Exclude(targetTools)
	logger.Infof("found %d tools in target; %d tools to be copied", len(targetTools), len(missing))
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
	logger.Infof("copied %d tools", len(missing))
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
	logger.Infof("using sync tools source: %v", sourceURL)
	return simplestreams.NewURLDataSource("sync tools source", sourceURL, utils.VerifySSLHostnames), nil
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
	resp, err := utils.GetValidatingHTTPClient().Get(tools.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Verify SHA-256 hash.
	var buf bytes.Buffer
	sha256, size, err := utils.ReadSHA256(io.TeeReader(resp.Body, &buf))
	if err != nil {
		return err
	}
	if tools.SHA256 == "" {
		logger.Warningf("no SHA-256 hash for %v", tools.SHA256)
	} else if sha256 != tools.SHA256 {
		return errors.Errorf("SHA-256 hash mismatch (%v/%v)", sha256, tools.SHA256)
	}
	sizeInKB := (size + 512) / 1024
	logger.Infof("uploading %v (%dkB) to environment", toolsName, sizeInKB)
	return u.UploadTools(toolsDir, stream, tools, buf.Bytes())
}

// UploadFunc is the type of Upload, which may be
// reassigned to control the behaviour of tools
// uploading.
type UploadFunc func(stor storage.Storage, stream string, forceVersion *version.Number, series ...string) (*coretools.Tools, error)

// Exported for testing.
var Upload UploadFunc = upload

// upload builds whatever version of github.com/juju/juju is in $GOPATH,
// uploads it to the given storage, and returns a Tools instance describing
// them. If forceVersion is not nil, the uploaded tools bundle will report
// the given version number; if any fakeSeries are supplied, additional copies
// of the built tools will be uploaded for use by machines of those series.
// Juju tools built for one series do not necessarily run on another, but this
// func exists only for development use cases.
func upload(stor storage.Storage, stream string, forceVersion *version.Number, fakeSeries ...string) (*coretools.Tools, error) {
	builtTools, err := BuildToolsTarball(forceVersion, stream)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(builtTools.Dir)
	logger.Debugf("Uploading tools for %v", fakeSeries)
	return syncBuiltTools(stor, stream, builtTools, fakeSeries...)
}

// cloneToolsForSeries copies the built tools tarball into a tarball for the specified
// stream and series and generates corresponding metadata.
func cloneToolsForSeries(toolsInfo *BuiltTools, stream string, series ...string) error {
	// Copy the tools to the target storage, recording a Tools struct for each one.
	var targetTools coretools.List
	targetTools = append(targetTools, &coretools.Tools{
		Version: toolsInfo.Version,
		Size:    toolsInfo.Size,
		SHA256:  toolsInfo.Sha256Hash,
	})
	putTools := func(vers version.Binary) (string, error) {
		name := envtools.StorageName(vers, stream)
		src := filepath.Join(toolsInfo.Dir, toolsInfo.StorageName)
		dest := filepath.Join(toolsInfo.Dir, name)
		destDir := filepath.Dir(dest)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return "", err
		}
		if err := utils.CopyFile(dest, src); err != nil {
			return "", err
		}
		// Append to targetTools the attributes required to write out tools metadata.
		targetTools = append(targetTools, &coretools.Tools{
			Version: vers,
			Size:    toolsInfo.Size,
			SHA256:  toolsInfo.Sha256Hash,
		})
		return name, nil
	}
	logger.Debugf("generating tarballs for %v", series)
	for _, series := range series {
		_, err := version.SeriesVersion(series)
		if err != nil {
			return err
		}
		if series != toolsInfo.Version.Series {
			fakeVersion := toolsInfo.Version
			fakeVersion.Series = series
			if _, err := putTools(fakeVersion); err != nil {
				return err
			}
		}
	}
	// The tools have been copied to a temp location from which they will be uploaded,
	// now write out the matching simplestreams metadata so that SyncTools can find them.
	metadataStore, err := filestorage.NewFileStorageWriter(toolsInfo.Dir)
	if err != nil {
		return err
	}
	logger.Debugf("generating tools metadata")
	return envtools.MergeAndWriteMetadata(metadataStore, stream, stream, targetTools, false)
}

// BuiltTools contains metadata for a tools tarball resulting from
// a call to BundleTools.
type BuiltTools struct {
	Version     version.Binary
	Dir         string
	StorageName string
	Sha256Hash  string
	Size        int64
}

// BuildToolsTarballFunc is a function which can build a tools tarball.
type BuildToolsTarballFunc func(forceVersion *version.Number, stream string) (*BuiltTools, error)

// Override for testing.
var BuildToolsTarball BuildToolsTarballFunc = buildToolsTarball

// buildToolsTarball bundles a tools tarball and places it in a temp directory in
// the expected tools path.
func buildToolsTarball(forceVersion *version.Number, stream string) (builtTools *BuiltTools, err error) {
	// TODO(rog) find binaries from $PATH when not using a development
	// version of juju within a $GOPATH.

	logger.Debugf("Building tools")
	// We create the entire archive before asking the environment to
	// start uploading so that we can be sure we have archived
	// correctly.
	f, err := ioutil.TempFile("", "juju-tgz")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer os.Remove(f.Name())
	toolsVersion, sha256Hash, err := envtools.BundleTools(f, forceVersion)
	if err != nil {
		return nil, err
	}
	fileInfo, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("cannot stat newly made tools archive: %v", err)
	}
	size := fileInfo.Size()
	logger.Infof("built tools %v (%dkB)", toolsVersion, (size+512)/1024)
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
	return &BuiltTools{
		Version:     toolsVersion,
		Dir:         baseToolsDir,
		StorageName: storageName,
		Size:        size,
		Sha256Hash:  sha256Hash,
	}, nil
}

// syncBuiltTools copies to storage a tools tarball and cloned copies for each series.
func syncBuiltTools(stor storage.Storage, stream string, builtTools *BuiltTools, fakeSeries ...string) (*coretools.Tools, error) {
	if err := cloneToolsForSeries(builtTools, stream, fakeSeries...); err != nil {
		return nil, err
	}
	syncContext := &SyncContext{
		Source:              builtTools.Dir,
		TargetToolsFinder:   StorageToolsFinder{stor},
		TargetToolsUploader: StorageToolsUploader{stor, false, false},
		AllVersions:         true,
		Stream:              stream,
		MajorVersion:        builtTools.Version.Major,
		MinorVersion:        -1,
	}
	logger.Debugf("uploading tools to cloud storage")
	err := SyncTools(syncContext)
	if err != nil {
		return nil, err
	}
	url, err := stor.URL(builtTools.StorageName)
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

// StorageToolsUplader is an implementation of ToolsUploader that
// writes tools to the provided storage and then writes merged
// metadata, optionally with mirrors.
type StorageToolsUploader struct {
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
	err := envtools.MergeAndWriteMetadata(u.Storage, toolsDir, stream, coretools.List{tools}, u.WriteMirrors)
	if err != nil {
		logger.Errorf("error writing tools metadata: %v", err)
		return err
	}
	return nil
}
