// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	domainrecovery "github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/recovery"
)

type validateSuite struct{}

func (s *validateSuite) TestValidateArchive(c *tc.C) {
	path, sum := writeArchive(c, validFiles())

	info, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.AgentVersion, tc.Equals, semversion.MustParse("4.1.0"))
	c.Check(info.ControllerUUID, tc.Equals, testControllerUUID)
	c.Check(info.ControllerName, tc.Equals, "source-ctrl")
	c.Check(info.ControllerModelUUID, tc.Equals, testControllerModelUUID)
	c.Check(info.HANodes, tc.Equals, int64(3))
	c.Check(info.CACert, tc.Equals, "source-ca-cert")
	c.Check(info.CAPrivateKey, tc.Equals, "source-ca-key")
	c.Check(info.CloudName, tc.Equals, "lxd")
	c.Check(info.CloudType, tc.Equals, "lxd")
	c.Check(info.Checksum, tc.Equals, sum)
	c.Check(info.Size > 0, tc.IsTrue)

	c.Assert(info.Models, tc.HasLen, 3)
	c.Check(info.Models[0].UUID, tc.Equals, testControllerModelUUID)
	c.Check(info.Models[0].ModelType, tc.Equals, "iaas")
	c.Check(info.Models[1].CloudType, tc.Equals, "lxd")
}

func (s *validateSuite) TestValidateArchiveChecksumMismatch(c *tc.C) {
	path, _ := writeArchive(c, validFiles())

	_, err := recovery.ValidateArchive(c.Context(), path, "deadbeef")
	c.Assert(err, tc.ErrorMatches, "archive checksum mismatch: expected sha256 .deadbeef., archive is .*")
}

func (s *validateSuite) TestValidateArchiveMissingMetadata(c *tc.C) {
	files := validFiles()
	delete(files, "juju-backup/metadata.json")
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches, "archive is missing juju-backup/metadata.json")
}

func (s *validateSuite) TestValidateArchiveMissingControllerDump(c *tc.C) {
	files := validFiles()
	delete(files, "juju-backup/dump/controller.yaml")
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches, "archive is missing juju-backup/dump/controller.yaml")
}

func (s *validateSuite) TestValidateArchiveMissingModelDump(c *tc.C) {
	files := validFiles()
	delete(files, "juju-backup/dump/models/"+testModelAUUID+".yaml")
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches,
		"archive is missing the database dump for model .workload-a. .*")
}

func (s *validateSuite) TestValidateArchiveUnknownModelDump(c *tc.C) {
	files := validFiles()
	files["juju-backup/dump/models/00000000-0000-0000-0000-000000000000.yaml"] = []byte("payload: {}\n")
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches,
		"archive contains a database dump for unknown model .*")
}

func (s *validateSuite) TestValidateArchiveRejectsTraversal(c *tc.C) {
	files := validFiles()
	files["juju-backup/../escape"] = []byte("x")
	path, _ := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, "")
	c.Assert(err, tc.ErrorMatches, "archive contains unsafe path .*")
}

func (s *validateSuite) TestValidateArchiveRejectsAbsolutePath(c *tc.C) {
	files := validFiles()
	files["/abs/path"] = []byte("x")
	path, _ := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, "")
	c.Assert(err, tc.ErrorMatches, "archive contains unsafe path .*")
}

func (s *validateSuite) TestValidateArchiveRejectsDuplicate(c *tc.C) {
	// The second entry cleans to the same path as metadata.json.
	files := validFiles()
	files["juju-backup/./metadata.json"] = []byte(metadataJSON("4.1.0"))
	path, _ := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, "")
	c.Assert(err, tc.ErrorMatches, "archive contains duplicate path .*")
}

func (s *validateSuite) TestValidateArchiveRejectsNonRegular(c *tc.C) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range []string{
		"juju-backup/metadata.json",
		"juju-backup/dump/controller.yaml",
	} {
		var data []byte
		if name == "juju-backup/metadata.json" {
			data = []byte(metadataJSON("4.1.0"))
		} else {
			data = []byte(controllerDump(modelRow(testControllerModelUUID, "controller", "cloud-lxd")))
		}
		c.Assert(tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg,
		}), tc.ErrorIsNil)
		_, err := tw.Write(data)
		c.Assert(err, tc.ErrorIsNil)
	}
	c.Assert(tw.WriteHeader(&tar.Header{
		Name: "juju-backup/evil", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd",
	}), tc.ErrorIsNil)
	c.Assert(tw.Close(), tc.ErrorIsNil)
	c.Assert(gz.Close(), tc.ErrorIsNil)

	path := filepath.Join(c.MkDir(), "juju-backup.tar.gz")
	c.Assert(os.WriteFile(path, buf.Bytes(), 0o600), tc.ErrorIsNil)

	_, err := recovery.ValidateArchive(c.Context(), path, "")
	c.Assert(err, tc.ErrorMatches, "archive entry .* is not a regular file")
}

func (s *validateSuite) TestValidateArchiveRejectsControllerUUIDMismatch(c *tc.C) {
	files := validFiles()
	// The dump's controller row records a different uuid than the
	// manifest: the archive is internally inconsistent.
	files["juju-backup/dump/controller.yaml"] = []byte(strings.Replace(
		controllerDump(
			modelRow(testControllerModelUUID, "controller", "cloud-lxd"),
			modelRow(testModelAUUID, "workload-a", "cloud-lxd"),
			modelRow(testModelBUUID, "workload-b", "cloud-lxd"),
		),
		"uuid: "+testControllerUUID, "uuid: deadbeef00-1111-2222-3333-444455556666", 1))
	archivePath, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), archivePath, sum)
	c.Assert(err, tc.ErrorMatches, "controller uuid mismatch: metadata records .*, controller row records .*")
}

func (s *validateSuite) TestValidateArchiveRejectsDuplicateModelUUID(c *tc.C) {
	files := validFiles()
	files["juju-backup/dump/controller.yaml"] = []byte(controllerDump(
		modelRow(testControllerModelUUID, "controller", "cloud-lxd"),
		modelRow(testModelAUUID, "workload-a", "cloud-lxd"),
		modelRow(testModelAUUID, "workload-a-again", "cloud-lxd"),
	))
	archivePath, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), archivePath, sum)
	c.Assert(err, tc.ErrorMatches, `dump records duplicate model uuid ".*"`)
}

func (s *validateSuite) TestValidateArchiveCAASWorkloadInventory(c *tc.C) {
	files := validFiles()
	// Turn workload-a into a CAAS model on the kubernetes cloud and
	// give it one archived application with a unit and a volume.
	files["juju-backup/dump/controller.yaml"] = []byte(controllerDump(
		modelRow(testControllerModelUUID, "controller", "cloud-lxd"),
		modelRow(testModelAUUID, "workload-a", "cloud-k8s"),
	))
	// workload-b is gone from the controller dump; its dump file must go
	// too or the inventory cross-check fails.
	delete(files, "juju-backup/dump/models/"+testModelBUUID+".yaml")
	// controllerDump hardcodes model_type_id 0; patch the one workload
	// row to caas (1).
	files["juju-backup/dump/controller.yaml"] = []byte(strings.Replace(
		string(files["juju-backup/dump/controller.yaml"]),
		"uuid: "+testModelAUUID+"\n    name: workload-a\n    cloud_uuid: cloud-k8s\n    model_type_id: 0",
		"uuid: "+testModelAUUID+"\n    name: workload-a\n    cloud_uuid: cloud-k8s\n    model_type_id: 1", 1))
	files["juju-backup/dump/models/"+testModelAUUID+".yaml"] = []byte(
		caasModelDump(
			[2]string{"11111111-cafe-0000-0000-000000000001", "gitlab"},
		))
	path, sum := writeArchive(c, files)

	info, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorIsNil)

	// The controller model and IAAS models carry no inventory; only the
	// CAAS workload model does.
	for _, m := range info.Models {
		if m.UUID != testModelAUUID {
			c.Check(m.Applications, tc.HasLen, 0)
		}
	}
	var workload *domainrecovery.ModelInfo
	for i := range info.Models {
		if info.Models[i].UUID == testModelAUUID {
			workload = &info.Models[i]
		}
	}
	c.Assert(workload, tc.NotNil)
	c.Assert(workload.Applications, tc.HasLen, 1)
	app := workload.Applications[0]
	c.Check(app.Name, tc.Equals, "gitlab")
	c.Check(app.UUID, tc.Equals, "11111111-cafe-0000-0000-000000000001")
	c.Assert(app.Units, tc.DeepEquals, []string{"gitlab/0"})
	c.Assert(app.FilesystemProviderIDs, tc.DeepEquals, []string{"gitlab-111111-0"})
}

func (s *validateSuite) TestValidateArchiveCAASInventorySkipsDeadEntities(c *tc.C) {
	files := validFiles()
	files["juju-backup/dump/controller.yaml"] = []byte(strings.Replace(
		controllerDump(
			modelRow(testControllerModelUUID, "controller", "cloud-lxd"),
			modelRow(testModelAUUID, "workload-a", "cloud-k8s"),
		),
		"uuid: "+testModelAUUID+"\n    name: workload-a\n    cloud_uuid: cloud-k8s\n    model_type_id: 0",
		"uuid: "+testModelAUUID+"\n    name: workload-a\n    cloud_uuid: cloud-k8s\n    model_type_id: 1", 1))
	// workload-b is gone from the controller dump; drop its dump file.
	delete(files, "juju-backup/dump/models/"+testModelBUUID+".yaml")
	deadDump := strings.Replace(caasModelDump(
		[2]string{"11111111-cafe-0000-0000-000000000001", "gitlab"},
	), "life_id: 0", "life_id: 2", 1)
	files["juju-backup/dump/models/"+testModelAUUID+".yaml"] = []byte(deadDump)
	path, sum := writeArchive(c, files)

	info, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorIsNil)

	var workload *domainrecovery.ModelInfo
	for i := range info.Models {
		if info.Models[i].UUID == testModelAUUID {
			workload = &info.Models[i]
		}
	}
	// The only application is dead: it is a pending removal in the
	// recovery summary, not substrate to verify.
	c.Assert(workload, tc.NotNil)
	c.Check(workload.Applications, tc.HasLen, 0)
}
