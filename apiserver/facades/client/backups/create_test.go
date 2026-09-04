// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	corebackups "github.com/juju/juju/core/backups"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	domainexport "github.com/juju/juju/domain/export"
	domainservicetesting "github.com/juju/juju/domain/services/testing"
	environsconfig "github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
)

type backupsSuite struct {
	domainservicetesting.DomainServicesSuite

	auditAuthorizer stubAuthorizer
}

func TestBackupsSuite(t *stdtesting.T) {
	tc.Run(t, &backupsSuite{})
}

func (s *backupsSuite) SetUpTest(c *tc.C) {
	s.DomainServicesSuite.SetUpTest(c)
	s.auditAuthorizer = stubAuthorizer{authClient: true}
}

// api builds a backups API wired to the suite's real domain services. Any
// non-nil field in overrides replaces the corresponding dependency, which
// lets error-path tests inject failures without rebuilding the suite.
func (s *backupsSuite) api(c *tc.C, backupDir string, overrides ...func(*apiDeps)) *API {
	controllerServices := s.ControllerDomainServices(c)

	cfg, err := environsconfig.New(environsconfig.UseDefaults, map[string]any{
		"backup-dir": backupDir,
		"uuid":       s.ControllerModelUUID.String(),
		"type":       "manual",
		"name":       "controller",
	})
	c.Assert(err, tc.ErrorIsNil)

	deps := apiDeps{
		controllerExport: controllerServices.ControllerExport(),
		modelServicesFor: func(_ context.Context, uuid coremodel.UUID) (ModelExportDomainServices, error) {
			return s.ModelDomainServices(c, uuid), nil
		},
		modelConfig:     stubModelConfig{cfg: cfg},
		controller:      controllerServices.Controller(),
		controllerNodes: controllerServices.ControllerNode(),
	}
	for _, override := range overrides {
		override(&deps)
	}

	api, err := NewAPI(
		&s.auditAuthorizer,
		names.NewMachineTag("0"),
		s.ControllerConfig.ControllerUUID(),
		s.ControllerModelUUID,
		s.dataDir(c), c.MkDir(),
		deps.controllerExport,
		deps.modelServicesFor,
		deps.modelConfig,
		deps.controller,
		deps.controllerNodes,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
	return api
}

// apiDeps holds the overridable dependencies of the backups API under test.
type apiDeps struct {
	controllerExport ControllerExportService
	modelServicesFor ModelServicesForFunc
	modelConfig      ModelConfigService
	controller       ControllerModelLister
	controllerNodes  ControllerNodeLister
}

// dataDir creates a data directory containing the objectstore and tools
// directories the file collector requires, and one file under the
// objectstore so the collected list is non-empty.
func (s *backupsSuite) dataDir(c *tc.C) string {
	dir := c.MkDir()
	for _, subdir := range []string{"objectstore", "tools"} {
		c.Assert(os.Mkdir(filepath.Join(dir, subdir), 0755), tc.ErrorIsNil)
	}
	c.Assert(os.WriteFile(filepath.Join(dir, "objectstore", "blob"), []byte("blob"), 0644), tc.ErrorIsNil)
	return dir
}

func (s *backupsSuite) TestCreate(c *tc.C) {
	backupDir := c.MkDir()
	api := s.api(c, backupDir)

	result, err := api.Create(c.Context(), params.BackupsCreateArgs{Notes: "test"})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(result.Notes, tc.Equals, "test")
	c.Check(result.FormatVersion, tc.Equals, int64(2))

	// The archive lands in the configured backup-dir.
	c.Check(filepath.Dir(result.Filename), tc.Equals, backupDir)

	controllerServices := s.ControllerDomainServices(c)
	modelUUIDs, err := controllerServices.Controller().GetModelNamespaces(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	controllerIDs, err := controllerServices.ControllerNode().GetControllerIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.HANodes, tc.Equals, int64(len(controllerIDs)))

	// Untar the archive and assert the dump entries.
	file, err := os.Open(result.Filename)
	c.Assert(err, tc.ErrorIsNil)
	defer file.Close()
	entries, contents := archiveContents(c, file)

	c.Assert(entries.Contains("juju-backup/dump/controller.yaml"), tc.IsTrue)
	for _, modelUUID := range modelUUIDs {
		c.Assert(entries.Contains("juju-backup/dump/models/"+modelUUID+".yaml"),
			tc.IsTrue, tc.Commentf("missing dump for model %s", modelUUID))
	}
	c.Assert(entries.Contains("juju-backup/metadata.json"), tc.IsTrue)
	c.Assert(entries.Contains("juju-backup/root.tar"), tc.IsTrue)

	// The controller dump envelope decodes and is stamped with the latest
	// controller export version.
	var controllerEnvelope domainexport.ControllerExport
	err = yaml.Unmarshal([]byte(contents["juju-backup/dump/controller.yaml"]), &controllerEnvelope)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerEnvelope.Version, tc.Equals, domainexport.LatestControllerExportVersion())
}

func (s *backupsSuite) TestCreateNotSuperuser(c *tc.C) {
	s.auditAuthorizer.hasErr = coreerrors.Forbidden

	_, err := s.api(c, c.MkDir()).Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, coreerrors.Forbidden)
}

// TestCreateModelServicesFailure verifies that a failure resolving a model's
// export services aborts Create and leaves no archive behind.
func (s *backupsSuite) TestCreateModelServicesFailure(c *tc.C) {
	backupDir := c.MkDir()
	boom := errors.New("model services unavailable")
	api := s.api(c, backupDir, func(d *apiDeps) {
		d.modelServicesFor = func(context.Context, coremodel.UUID) (ModelExportDomainServices, error) {
			return nil, boom
		}
	})

	_, err := api.Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, boom)

	entries, err := os.ReadDir(backupDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}

// TestCreateModelNamespacesFailure verifies that a failure listing the
// registered model namespaces aborts Create and leaves no archive behind.
func (s *backupsSuite) TestCreateModelNamespacesFailure(c *tc.C) {
	backupDir := c.MkDir()
	boom := errors.New("cannot list models")
	api := s.api(c, backupDir, func(d *apiDeps) {
		d.controller = stubModelLister{err: boom}
	})

	_, err := api.Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIs, boom)

	entries, err := os.ReadDir(backupDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 0)
}

// TestCreateNoModels verifies that a controller with no model namespaces
// still produces a valid archive containing only the controller dump.
func (s *backupsSuite) TestCreateNoModels(c *tc.C) {
	backupDir := c.MkDir()
	api := s.api(c, backupDir, func(d *apiDeps) {
		d.controller = stubModelLister{uuids: []string{}}
	})

	result, err := api.Create(c.Context(), params.BackupsCreateArgs{})
	c.Assert(err, tc.ErrorIsNil)

	file, err := os.Open(result.Filename)
	c.Assert(err, tc.ErrorIsNil)
	defer file.Close()
	entries, _ := archiveContents(c, file)
	c.Check(entries.Contains("juju-backup/dump/controller.yaml"), tc.IsTrue)
}

func archiveContents(c *tc.C, r io.Reader) (set.Strings, map[string]string) {
	names := set.NewStrings()
	contents := make(map[string]string)

	archive, err := corebackups.NewArchiveDataReader(r)
	c.Assert(err, tc.ErrorIsNil)

	tr := tar.NewReader(archive.NewBuffer())
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		c.Assert(err, tc.ErrorIsNil)
		if header.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			c.Assert(err, tc.ErrorIsNil)
			contents[header.Name] = string(data)
		}
		names.Add(header.Name)
	}
	return names, contents
}

// stubAuthorizer is an Authorizer whose HasPermission outcome is tunable.
type stubAuthorizer struct {
	authClient bool
	hasErr     error

	// target is the tag on which HasPermission was last invoked.
	target names.Tag
}

func (s *stubAuthorizer) GetAuthTag() names.Tag      { return names.NewUserTag("admin") }
func (s *stubAuthorizer) AuthController() bool       { return false }
func (s *stubAuthorizer) AuthMachineAgent() bool     { return false }
func (s *stubAuthorizer) AuthApplicationAgent() bool { return false }
func (s *stubAuthorizer) AuthModelAgent() bool       { return false }
func (s *stubAuthorizer) AuthUnitAgent() bool        { return false }
func (s *stubAuthorizer) AuthOwner(names.Tag) bool   { return false }
func (s *stubAuthorizer) AuthClient() bool           { return s.authClient }
func (s *stubAuthorizer) HasPermission(_ context.Context, _ permission.Access, target names.Tag) error {
	s.target = target
	return s.hasErr
}
func (s *stubAuthorizer) EntityHasPermission(_ context.Context, _ names.Tag, _ permission.Access, _ names.Tag) error {
	return nil
}

type stubModelConfig struct {
	cfg *environsconfig.Config
}

func (s stubModelConfig) ModelConfig(context.Context) (*environsconfig.Config, error) {
	return s.cfg, nil
}

// stubModelLister is a ControllerModelLister returning fixed UUIDs or an
// error.
type stubModelLister struct {
	uuids []string
	err   error
}

func (s stubModelLister) GetModelNamespaces(context.Context) ([]string, error) {
	return s.uuids, s.err
}
