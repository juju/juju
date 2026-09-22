// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	corebackups "github.com/juju/juju/core/backups"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	domainexport "github.com/juju/juju/domain/export"
	environsconfig "github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type backupsSuite struct {
	testhelpers.IsolationSuite

	controllerUUID      string
	controllerModelUUID coremodel.UUID
	modelUUID           string
	dataDirPath         string
	logDirPath          string

	authorizer       *MockAuthorizer
	controllerExport *MockControllerExportService
	modelConfig      *MockModelConfigService
	controller       *MockControllerModelLister
	controllerNodes  *MockControllerNodeLister
	modelServices    *MockModelExportDomainServices
	modelExport      *MockModelExportService

	// Captured by the patched core archive creation.
	archiveFilename string
	archiveData     []byte
	createdMeta     *corebackups.Metadata
	createdArgs     corebackups.CreateArgs
	createdDumps    []string
}

func TestBackupsSuite(t *stdtesting.T) {
	tc.Run(t, &backupsSuite{})
}

func (s *backupsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.controllerUUID = uuid.MustNewUUID().String()
	s.controllerModelUUID = coremodel.UUID(uuid.MustNewUUID().String())
	s.modelUUID = uuid.MustNewUUID().String()
	// The paths are only assembled into corebackups.Paths: the file
	// walk is patched, so nothing reads them. Real empty directories
	// make any accidental real walk fail loudly.
	s.dataDirPath = c.MkDir()
	s.logDirPath = c.MkDir()
}

// setupMocks creates gomock mocks for every dependency of the backups
// API. The facade is tested against mocks: no database is involved.
func (s *backupsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = NewMockAuthorizer(ctrl)
	s.controllerExport = NewMockControllerExportService(ctrl)
	s.modelConfig = NewMockModelConfigService(ctrl)
	s.controller = NewMockControllerModelLister(ctrl)
	s.controllerNodes = NewMockControllerNodeLister(ctrl)
	s.modelServices = NewMockModelExportDomainServices(ctrl)
	s.modelExport = NewMockModelExportService(ctrl)

	c.Cleanup(func() {
		s.authorizer = nil
		s.controllerExport = nil
		s.modelConfig = nil
		s.controller = nil
		s.controllerNodes = nil
		s.modelServices = nil
		s.modelExport = nil
	})

	return ctrl
}

// newAPI returns a backups API wired to the suite's mocked dependencies
// and the given model services resolver.
func (s *backupsSuite) newAPI(c *tc.C, modelServicesFor ModelServicesForFunc) *API {
	api, err := NewAPI(
		s.authorizer,
		names.NewMachineTag("0"),
		s.controllerUUID,
		s.controllerModelUUID,
		s.dataDirPath,
		s.logDirPath,
		s.controllerExport,
		modelServicesFor,
		s.modelConfig,
		s.controller,
		s.controllerNodes,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
	return api
}

// modelServicesFor returns a ModelServicesForFunc handing back the
// suite's model export services mock for every model UUID.
func (s *backupsSuite) modelServicesFor() ModelServicesForFunc {
	return ModelServicesForFunc(func(context.Context, coremodel.UUID) (ModelExportDomainServices, error) {
		return s.modelServices, nil
	})
}

// expectSuperuser primes the authorizer mock so NewAPI and Create accept
// the client, with the superuser check targeting the controller tag.
func (s *backupsSuite) expectSuperuser() {
	s.authorizer.EXPECT().AuthClient().Return(true).AnyTimes()
	s.authorizer.EXPECT().HasPermission(
		gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID),
	).Return(nil).AnyTimes()
}

// expectModelConfig primes the model config mock to resolve the backup
// directory to backupDir.
func (s *backupsSuite) expectModelConfig(c *tc.C, backupDir string) {
	cfg, err := environsconfig.New(environsconfig.UseDefaults, map[string]any{
		"backup-dir": backupDir,
		"uuid":       s.controllerModelUUID.String(),
		"type":       "manual",
		"name":       "controller",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.modelConfig.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)
}

// expectControllerExport primes the controller node lister to report two
// controller machines, and the controller export mock to return an export
// stamped with the latest controller export version.
func (s *backupsSuite) expectControllerExport() {
	s.controllerNodes.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"0", "1"}, nil)
	s.controllerExport.EXPECT().Export(gomock.Any()).Return(&domainexport.ControllerExport{
		Version: domainexport.LatestControllerExportVersion(),
	}, nil)
}

// expectFilesToBackUp patches the core file collection to return one
// file, asserting the paths the facade assembled from its fields and
// the model config.
func (s *backupsSuite) expectFilesToBackUp(c *tc.C, backupDir string) []string {
	blob := filepath.Join(c.MkDir(), "blob")
	c.Assert(os.WriteFile(blob, []byte("blob"), 0644), tc.ErrorIsNil)

	s.PatchValue(&corebackups.GetFilesToBackUp, func(rootDir string, paths *corebackups.Paths) ([]string, error) {
		c.Check(rootDir, tc.Equals, "")
		c.Check(*paths, tc.DeepEquals, corebackups.Paths{
			BackupDir: backupDir,
			DataDir:   s.dataDirPath,
			LogsDir:   s.logDirPath,
		})
		return []string{blob}, nil
	})
	return []string{blob}
}

// expectArchiveCreation patches the core archive creation to capture the
// metadata and arguments the facade assembled, reading the staged dumps
// while their readers are still open.
func (s *backupsSuite) expectArchiveCreation(c *tc.C, backupDir string) {
	s.archiveFilename = filepath.Join(backupDir, "juju-backup-test.tar.gz")
	s.archiveData = []byte("test archive data")
	s.PatchValue(&corebackups.Create, func(meta *corebackups.Metadata, args corebackups.CreateArgs) (string, error) {
		s.createdMeta = meta
		s.createdArgs = args
		s.createdDumps = nil
		for _, entry := range args.DumpEntries {
			data, err := io.ReadAll(entry.Reader)
			c.Assert(err, tc.ErrorIsNil)
			s.createdDumps = append(s.createdDumps, string(data))
		}
		// The facade reads the archive back and removes it, so the
		// stub must write a real file.
		err := os.WriteFile(s.archiveFilename, s.archiveData, 0600)
		c.Assert(err, tc.ErrorIsNil)
		return s.archiveFilename, nil
	})
}

// assertNoArchive verifies that Create left no archive or staged dumps
// behind in backupDir.
func (s *backupsSuite) assertNoArchive(c *tc.C, backupDir string) {
	entries, err := os.ReadDir(backupDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}

// TestNewAPINotClient verifies that NewAPI rejects non-client callers.
func (s *backupsSuite) TestNewAPINotClient(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().AuthClient().Return(false)

	api, err := NewAPI(
		s.authorizer,
		names.NewMachineTag("0"),
		s.controllerUUID,
		s.controllerModelUUID,
		s.dataDirPath,
		s.logDirPath,
		s.controllerExport,
		s.modelServicesFor(),
		s.modelConfig,
		s.controller,
		s.controllerNodes,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	c.Check(api, tc.IsNil)
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *backupsSuite) TestCreate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	files := s.expectFilesToBackUp(c, backupDir)
	s.expectArchiveCreation(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.expectControllerExport()
	s.controller.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{s.modelUUID}, nil)
	s.modelServices.EXPECT().Export().Return(s.modelExport)
	s.modelExport.EXPECT().Export(gomock.Any()).Return(&domainexport.ModelExport{
		Version: domainexport.LatestSupportedPayloadVersion(),
	}, nil)

	result, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{Notes: "test"})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(result.Notes, tc.Equals, "test")
	c.Check(result.FormatVersion, tc.Equals, int64(2))
	c.Check(result.HANodes, tc.Equals, int64(2))
	c.Check(result.Filename, tc.Equals, filepath.Base(s.archiveFilename))

	// The archive is staged under the returned one-shot download id,
	// and the original archive file is gone.
	stagedPath, err := corebackups.OneShotArchivePath(backupDir, result.ID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.ID, tc.Not(tc.Equals), "")
	staged, err := os.ReadFile(stagedPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(staged, tc.DeepEquals, s.archiveData)
	_, err = os.Stat(s.archiveFilename)
	c.Assert(err, tc.Satisfies, os.IsNotExist)

	// The metadata is assembled from the request and the services.
	c.Check(s.createdMeta.Notes, tc.Equals, "test")
	c.Check(s.createdMeta.Origin.Model, tc.Equals, s.controllerModelUUID.String())
	c.Check(s.createdMeta.Origin.Machine, tc.Equals, "0")
	c.Check(s.createdMeta.Controller.UUID, tc.Equals, s.controllerUUID)
	c.Check(s.createdMeta.Controller.MachineID, tc.Equals, "0")
	c.Check(s.createdMeta.Controller.HANodes, tc.Equals, int64(2))

	// The archive is created in the configured backup directory, with
	// the collected files and one dump per database: the controller's
	// plus one per registered model namespace.
	c.Check(s.createdArgs.DestinationDir, tc.Equals, backupDir)
	c.Check(s.createdArgs.FilesToBackUp, tc.DeepEquals, files)
	c.Check(s.createdArgs.Clock, tc.NotNil)

	c.Assert(s.createdArgs.DumpEntries, tc.HasLen, 2)
	c.Check(s.createdArgs.DumpEntries[0].Name, tc.Equals, "controller.yaml")
	c.Check(s.createdArgs.DumpEntries[1].Name, tc.Equals, "models/"+s.modelUUID+".yaml")

	// The staged dumps carry the exports the services returned.
	var controllerEnvelope domainexport.ControllerExport
	err = yaml.Unmarshal([]byte(s.createdDumps[0]), &controllerEnvelope)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerEnvelope.Version, tc.Equals, domainexport.LatestControllerExportVersion())

	var modelEnvelope domainexport.ModelExport
	err = yaml.Unmarshal([]byte(s.createdDumps[1]), &modelEnvelope)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelEnvelope.Version, tc.Equals, domainexport.LatestSupportedPayloadVersion())

	// The staged dumps are cleaned up once archive creation returns, and
	// the only thing left is the archive staged for download.
	entries, err := os.ReadDir(corebackups.OneShotDir(backupDir))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entries, tc.HasLen, 1)
	c.Check(entries[0].Name(), tc.Equals, result.ID+".tar.gz")
}

func (s *backupsSuite) TestCreateNotSuperuser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().AuthClient().Return(true).AnyTimes()
	s.authorizer.EXPECT().HasPermission(
		gomock.Any(), permission.SuperuserAccess, gomock.Any(),
	).Return(coreerrors.Forbidden)

	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, coreerrors.Forbidden)
}

// TestCreateNoDownload verifies that a request using the deprecated
// no-download flag is rejected with a clear error: those semantics no
// longer exist, and a silent success would discard the archive.
func (s *backupsSuite) TestCreateNoDownload(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectSuperuser()

	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(),
		params.BackupsCreateArgs{NoDownload: true})
	c.Assert(err, tc.ErrorMatches,
		"keeping archives on the controller is no longer supported; the archive is always downloaded")
	// The rejection is classified so the client can tell it apart from
	// a transport failure.
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}

// TestCreateGetFilesFailure verifies that a failure collecting the files
// to back up aborts Create and leaves no archive behind.
func (s *backupsSuite) TestCreateGetFilesFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	boom := errors.New("cannot list files")

	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.PatchValue(&corebackups.GetFilesToBackUp, func(string, *corebackups.Paths) ([]string, error) {
		return nil, boom
	})

	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, boom)

	s.assertNoArchive(c, backupDir)
}

// TestCreateModelServicesFailure verifies that a failure resolving a model's
// export services aborts Create and leaves no archive behind.
func (s *backupsSuite) TestCreateModelServicesFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	boom := errors.New("model services unavailable")
	modelServicesFor := ModelServicesForFunc(func(context.Context, coremodel.UUID) (ModelExportDomainServices, error) {
		return nil, boom
	})

	s.expectFilesToBackUp(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.expectControllerExport()
	s.controller.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{s.modelUUID}, nil)

	_, err := s.newAPI(c, modelServicesFor).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, boom)

	s.assertNoArchive(c, backupDir)
}

// TestCreateModelNamespacesFailure verifies that a failure listing the
// registered model namespaces aborts Create and leaves no archive behind.
func (s *backupsSuite) TestCreateModelNamespacesFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	boom := errors.New("cannot list models")

	s.expectFilesToBackUp(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.expectControllerExport()
	s.controller.EXPECT().GetModelNamespaces(gomock.Any()).Return(nil, boom)

	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, boom)

	s.assertNoArchive(c, backupDir)
}

// TestCreateModelExportFailure verifies that a failure exporting a model's
// database aborts Create and leaves no archive behind.
func (s *backupsSuite) TestCreateModelExportFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	boom := errors.New("cannot export model")

	s.expectFilesToBackUp(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.expectControllerExport()
	s.controller.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{s.modelUUID}, nil)
	s.modelServices.EXPECT().Export().Return(s.modelExport)
	s.modelExport.EXPECT().Export(gomock.Any()).Return(nil, boom)

	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, boom)

	s.assertNoArchive(c, backupDir)
}

// TestCreateContextCancelled verifies that a cancelled context aborts
// Create between model exports, leaving no archive behind.
func (s *backupsSuite) TestCreateContextCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	s.expectFilesToBackUp(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.expectControllerExport()
	s.controller.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{s.modelUUID}, nil)

	_, err := s.newAPI(c, s.modelServicesFor()).Create(ctx, params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, context.Canceled)

	// No model export runs after cancellation.
	s.assertNoArchive(c, backupDir)
}

// TestCreateStageDumpsFailure verifies that a failure staging the
// database dumps aborts Create and leaves no archive behind. Staging
// happens in the backup directory, so a backup dir that is a regular
// file makes it fail regardless of privileges.
func (s *backupsSuite) TestCreateStageDumpsFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := filepath.Join(c.MkDir(), "backup-dir")
	c.Assert(os.WriteFile(backupDir, nil, 0644), tc.ErrorIsNil)

	s.expectFilesToBackUp(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.expectControllerExport()
	s.controller.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{s.modelUUID}, nil)
	s.modelServices.EXPECT().Export().Return(s.modelExport)
	s.modelExport.EXPECT().Export(gomock.Any()).Return(&domainexport.ModelExport{}, nil)
	s.PatchValue(&corebackups.Create, func(*corebackups.Metadata, corebackups.CreateArgs) (string, error) {
		return "", errors.New("archive must not be created")
	})

	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorMatches, ".*not a directory")
}

// TestCreateNoModels verifies that a controller with no model namespaces
// still produces an archive containing only the controller dump.
func (s *backupsSuite) TestCreateNoModels(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()

	s.expectFilesToBackUp(c, backupDir)
	s.expectArchiveCreation(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.expectControllerExport()
	s.controller.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{}, nil)

	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.createdArgs.DumpEntries, tc.HasLen, 1)
	c.Check(s.createdArgs.DumpEntries[0].Name, tc.Equals, "controller.yaml")

	// Only the staged one-shot archive is left behind.
	entries, err := os.ReadDir(corebackups.OneShotDir(backupDir))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entries, tc.HasLen, 1)
}

// TestCreateControllerExportFailure verifies that a failure exporting the
// controller database aborts Create and leaves no archive behind.
func (s *backupsSuite) TestCreateControllerExportFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	boom := errors.New("cannot export controller")

	s.expectFilesToBackUp(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.controllerNodes.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"0"}, nil)
	s.controllerExport.EXPECT().Export(gomock.Any()).Return(nil, boom)

	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, boom)

	s.assertNoArchive(c, backupDir)
}

// TestCreateModelConfigFailure verifies that a failure reading the model
// config aborts Create and leaves no archive behind.
func (s *backupsSuite) TestCreateModelConfigFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	boom := errors.New("cannot read model config")

	s.expectSuperuser()
	s.modelConfig.EXPECT().ModelConfig(gomock.Any()).Return(nil, boom)

	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, boom)

	s.assertNoArchive(c, backupDir)
}

// TestCreateControllerNodesFailure verifies that a failure listing
// controller machine IDs aborts Create before any archive is created.
func (s *backupsSuite) TestCreateControllerNodesFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()

	s.expectFilesToBackUp(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.controllerNodes.EXPECT().GetControllerIDs(gomock.Any()).Return(nil, controllernodeerrors.EmptyControllerIDs)

	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyControllerIDs)

	s.assertNoArchive(c, backupDir)
}

// TestCreateNoSpaceFailure verifies that insufficient disk space aborts
// Create and cleans up the staged dumps.
func (s *backupsSuite) TestCreateNoSpaceFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	boom := errors.New("insufficient disk space")

	s.expectFilesToBackUp(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.expectControllerExport()
	s.controller.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{}, nil)
	s.PatchValue(&corebackups.CheckSpaceFor, func(string, int64) error {
		return boom
	})

	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, boom)

	s.assertNoArchive(c, backupDir)
}

// TestCreateMissingBackupFile verifies that a file vanishing between
// collection and sizing is excluded from the space estimate without
// aborting Create.
func (s *backupsSuite) TestCreateMissingBackupFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	s.expectArchiveCreation(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.expectControllerExport()
	s.controller.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{}, nil)
	s.PatchValue(&corebackups.GetFilesToBackUp, func(string, *corebackups.Paths) ([]string, error) {
		return []string{filepath.Join(c.MkDir(), "vanished")}, nil
	})

	result, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Filename, tc.Equals, filepath.Base(s.archiveFilename))
	stagedPath, err := corebackups.OneShotArchivePath(backupDir, result.ID)
	c.Assert(err, tc.ErrorIsNil)
	staged, err := os.ReadFile(stagedPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(staged, tc.DeepEquals, s.archiveData)
}

// TestCreateArchiveFailure verifies that a failure creating the archive
// aborts Create and cleans up the staged dumps.
func (s *backupsSuite) TestCreateArchiveFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	boom := errors.New("cannot create archive")

	s.expectFilesToBackUp(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.expectControllerExport()
	s.controller.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{s.modelUUID}, nil)
	s.modelServices.EXPECT().Export().Return(s.modelExport)
	s.modelExport.EXPECT().Export(gomock.Any()).Return(&domainexport.ModelExport{
		Version: domainexport.LatestSupportedPayloadVersion(),
	}, nil)
	s.PatchValue(&corebackups.Create, func(*corebackups.Metadata, corebackups.CreateArgs) (string, error) {
		return "", boom
	})
	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, boom)

	s.assertNoArchive(c, backupDir)
}

// TestCreateArchiveStagingFailure verifies that a failure staging the
// created archive for one-shot download fails Create and leaves no
// archive behind.
func (s *backupsSuite) TestCreateArchiveStagingFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	missing := filepath.Join(backupDir, "vanished.tar.gz")

	s.expectFilesToBackUp(c, backupDir)
	s.expectSuperuser()
	s.expectModelConfig(c, backupDir)
	s.expectControllerExport()
	s.controller.EXPECT().GetModelNamespaces(gomock.Any()).Return([]string{}, nil)
	s.PatchValue(&corebackups.Create, func(*corebackups.Metadata, corebackups.CreateArgs) (string, error) {
		return missing, nil
	})

	_, err := s.newAPI(c, s.modelServicesFor()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorMatches, `staging backup archive for download.*`)

	// The one-shot download dir exists but holds no archive.
	entries, err := os.ReadDir(corebackups.OneShotDir(backupDir))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}
