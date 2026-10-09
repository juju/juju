// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/semversion"
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
	// Give the CAAS workload model one archived application with a unit
	// and a volume.
	files := caasFiles(caasModelDump(
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
	workload := findModel(c, info, testModelAUUID)
	c.Assert(workload.Applications, tc.HasLen, 1)
	app := workload.Applications[0]
	c.Check(app.Name, tc.Equals, "gitlab")
	c.Check(app.UUID, tc.Equals, "11111111-cafe-0000-0000-000000000001")
	c.Assert(app.Units, tc.DeepEquals, []string{"gitlab/0"})
	c.Assert(app.FilesystemProviderIDs, tc.DeepEquals, []string{"gitlab-111111-0"})
}

func (s *validateSuite) TestValidateArchiveCAASInventorySkipsDeadEntities(c *tc.C) {
	deadDump := strings.Replace(caasModelDump(
		[2]string{"11111111-cafe-0000-0000-000000000001", "gitlab"},
	), "life_id: 0", "life_id: 2", 1)
	files := caasFiles(deadDump)
	path, sum := writeArchive(c, files)

	info, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorIsNil)

	// The only application is dead: it is a pending removal in the
	// recovery summary, not substrate to verify.
	workload := findModel(c, info, testModelAUUID)
	c.Check(workload.Applications, tc.HasLen, 0)
}

// TestValidateArchiveCreatedByBackup is the producer-consumer round
// trip: an archive written by the real backup Create — staged manifest,
// directory entries, hex checksum recorded in the metadata — must pass
// recovery validation, so a layout or manifest-format drift in the
// producer fails here instead of making every new backup unrecoverable.
func (s *validateSuite) TestValidateArchiveCreatedByBackup(c *tc.C) {
	destDir := c.MkDir()
	dataFile := filepath.Join(c.MkDir(), "jujud")
	c.Assert(os.WriteFile(dataFile, []byte("agent binary"), 0o644), tc.ErrorIsNil)

	meta := corebackups.NewMetadata(time.Date(2026, time.October, 8, 12, 0, 0, 0, time.UTC))
	meta.Origin = corebackups.Origin{
		Model:    testControllerModelUUID,
		Machine:  "0",
		Hostname: "myhost",
		Version:  semversion.MustParse("4.1.0"),
	}
	meta.Controller = corebackups.ControllerMetadata{
		UUID:    testControllerUUID,
		HANodes: 1,
	}
	filename, err := corebackups.Create(meta, corebackups.CreateArgs{
		DestinationDir: destDir,
		Clock:          clock.WallClock,
		FilesToBackUp:  []string{dataFile},
		DumpEntries: []corebackups.DumpEntry{{
			Name: "controller.yaml",
			Reader: strings.NewReader(controllerDump(
				modelRow(testControllerModelUUID, "controller", "cloud-lxd"),
				modelRow(testModelAUUID, "workload-a", "cloud-lxd"),
			)),
		}, {
			Name:   "models/" + testControllerModelUUID + ".yaml",
			Reader: strings.NewReader("payload: {}\n"),
		}, {
			Name:   "models/" + testModelAUUID + ".yaml",
			Reader: strings.NewReader("payload: {}\n"),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	// The checksum recorded in the metadata during creation is the
	// operator-supplied trust anchor for validation.
	info, err := recovery.ValidateArchive(c.Context(), filename, meta.Checksum())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.AgentVersion, tc.Equals, semversion.MustParse("4.1.0"))
	c.Check(info.ControllerUUID, tc.Equals, testControllerUUID)
	c.Check(info.ControllerModelUUID, tc.Equals, testControllerModelUUID)
	c.Check(info.Checksum, tc.Equals, meta.Checksum())
	c.Check(meta.ChecksumFormat(), tc.Equals, corebackups.ChecksumFormatSHA256)
	c.Check(info.CloudType, tc.Equals, "lxd")
	c.Assert(info.Models, tc.HasLen, 2)
	family, err := info.ModelFamily()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(family, tc.Equals, "iaas")
}

func (s *validateSuite) TestValidateArchiveNoControllerRow(c *tc.C) {
	files := dumpOnlyFiles(rawControllerDump("", validCloudRows(),
		rawModelRow(testControllerModelUUID, "controller", "cloud-lxd", "0")))
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches,
		"juju-backup/dump/controller.yaml records no controller row")
}

// The CA-material branch matters most among the controller row checks:
// a regression there means bootstrapping a replacement controller with
// empty CA material.
func (s *validateSuite) TestValidateArchiveMissingCAMaterial(c *tc.C) {
	row := "  - uuid: " + testControllerUUID + "\n" +
		"    model_uuid: " + testControllerModelUUID + "\n"
	files := dumpOnlyFiles(rawControllerDump(row, validCloudRows(),
		rawModelRow(testControllerModelUUID, "controller", "cloud-lxd", "0")))
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches,
		"juju-backup/dump/controller.yaml records no controller CA material")
}

func (s *validateSuite) TestValidateArchiveControllerModelUUIDMismatch(c *tc.C) {
	row := "  - uuid: " + testControllerUUID + "\n" +
		"    model_uuid: deadbeef00-aaaa-2222-3333-444455556666\n" +
		"    ca_cert: source-ca-cert\n" +
		"    ca_private_key: source-ca-key\n"
	files := dumpOnlyFiles(rawControllerDump(row, validCloudRows(),
		rawModelRow(testControllerModelUUID, "controller", "cloud-lxd", "0")))
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches,
		"controller model mismatch: metadata records .*, controller row records .*")
}

func (s *validateSuite) TestValidateArchiveUnknownCloudUUID(c *tc.C) {
	files := dumpOnlyFiles(rawControllerDump(validControllerRow(), validCloudRows(),
		rawModelRow(testControllerModelUUID, "controller", "cloud-gone", "0")))
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches,
		`model "controller" references unknown cloud "cloud-gone"`)
}

func (s *validateSuite) TestValidateArchiveUnknownModelType(c *tc.C) {
	files := dumpOnlyFiles(rawControllerDump(validControllerRow(), validCloudRows(),
		rawModelRow(testControllerModelUUID, "controller", "cloud-lxd", "9")))
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches,
		`model "controller" has unknown model type`)
}

func (s *validateSuite) TestValidateArchiveUnknownCloudType(c *tc.C) {
	clouds := "  - uuid: cloud-lxd\n    name: lxd\n    cloud_type_id: 9\n"
	files := dumpOnlyFiles(rawControllerDump(validControllerRow(), clouds,
		rawModelRow(testControllerModelUUID, "controller", "cloud-lxd", "0")))
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches,
		`model "controller" has unknown cloud type`)
}

func (s *validateSuite) TestValidateArchiveNoModels(c *tc.C) {
	files := dumpOnlyFiles(rawControllerDump(validControllerRow(), validCloudRows(), ""))
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches,
		"juju-backup/dump/controller.yaml records no models")
}

func (s *validateSuite) TestValidateArchiveControllerModelNotInModelTable(c *tc.C) {
	files := dumpOnlyFiles(rawControllerDump(validControllerRow(), validCloudRows(),
		rawModelRow(testModelAUUID, "workload-a", "cloud-lxd", "0")))
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches,
		`controller model ".*" not found in juju-backup/dump/controller.yaml`)
}

// The dying(1)-included / dead(2)-excluded boundary decides which
// workload objects the preflight reports as surviving substrate: dying
// objects still exist in the cluster, dead ones are pending removal.
func (s *validateSuite) TestValidateArchiveCAASInventoryLifeBoundary(c *tc.C) {
	for i, test := range []struct {
		about      string
		anchor     string
		life       int
		wantUnits  []string
		wantClaims []string
	}{{
		about:      "dying unit still counted",
		anchor:     "name: gitlab/0\n    life_id: 0",
		life:       1,
		wantUnits:  []string{"gitlab/0"},
		wantClaims: []string{"gitlab-111111-0"},
	}, {
		about:  "dead unit dropped, its volume unattributed",
		anchor: "name: gitlab/0\n    life_id: 0",
		life:   2,
	}, {
		about:      "dying filesystem still counted",
		anchor:     "filesystem_id: gitlab-vol\n    life_id: 0",
		life:       1,
		wantUnits:  []string{"gitlab/0"},
		wantClaims: []string{"gitlab-111111-0"},
	}, {
		about:     "dead filesystem dropped",
		anchor:    "filesystem_id: gitlab-vol\n    life_id: 0",
		life:      2,
		wantUnits: []string{"gitlab/0"},
	}, {
		about:      "dying attachment still counted",
		anchor:     "net_node_uuid: node-gitlab\n    life_id: 0",
		life:       1,
		wantUnits:  []string{"gitlab/0"},
		wantClaims: []string{"gitlab-111111-0"},
	}, {
		about:     "dead attachment dropped",
		anchor:    "net_node_uuid: node-gitlab\n    life_id: 0",
		life:      2,
		wantUnits: []string{"gitlab/0"},
	}} {
		c.Logf("%d: %s", i, test.about)
		dump := strings.Replace(
			caasModelDump([2]string{"11111111-cafe-0000-0000-000000000001", "gitlab"}),
			test.anchor,
			strings.Replace(test.anchor, "life_id: 0", fmt.Sprintf("life_id: %d", test.life), 1),
			1)
		path, sum := writeArchive(c, caasFiles(dump))

		info, err := recovery.ValidateArchive(c.Context(), path, sum)
		c.Assert(err, tc.ErrorIsNil)
		workload := findModel(c, info, testModelAUUID)
		c.Assert(workload.Applications, tc.HasLen, 1)
		app := workload.Applications[0]
		c.Check(app.Units, tc.DeepEquals, test.wantUnits, tc.Commentf(test.about))
		c.Check(app.FilesystemProviderIDs, tc.DeepEquals, test.wantClaims, tc.Commentf(test.about))
	}
}

// A metadata.json of exactly the reader's bound must still pass; one
// byte over it must fail the read.
func (s *validateSuite) TestValidateArchiveMetadataSizeBoundary(c *tc.C) {
	// maxMetadataSize is 4 MiB; pad the metadata's Notes to land the
	// file exactly on it.
	const maxMetadata = 4 << 20
	meta := metadataJSON("4.1.0")
	pad := strings.Repeat("a", maxMetadata-len(meta))
	padded := strings.Replace(meta, `"Notes":"",`, `"Notes":"`+pad+`",`, 1)
	c.Assert(len(padded), tc.Equals, maxMetadata)

	files := validFiles()
	files["juju-backup/metadata.json"] = []byte(padded)
	path, sum := writeArchive(c, files)

	info, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.AgentVersion, tc.Equals, semversion.MustParse("4.1.0"))

	files = validFiles()
	files["juju-backup/metadata.json"] = []byte(
		strings.Replace(meta, `"Notes":"",`, `"Notes":"`+pad+`a",`, 1))
	path, sum = writeArchive(c, files)

	_, err = recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches,
		`reading "juju-backup/metadata.json": file exceeds 4194304 bytes`)
}

// The reader checks cancellation per entry: a cancelled context must
// surface before the checksum stage.
func (s *validateSuite) TestValidateArchiveCancelled(c *tc.C) {
	path, _ := writeArchive(c, validFiles())

	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	// The wrong expected checksum would fail at the checksum stage;
	// the cancellation must surface first.
	_, err := recovery.ValidateArchive(ctx, path, "deadbeef")
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

// Model dumps must sit directly under dump/models/: an empty or nested
// model name is not a dump the inventory can attribute.
func (s *validateSuite) TestValidateArchiveUnexpectedModelDumpPath(c *tc.C) {
	for _, name := range []string{
		"juju-backup/dump/models/.yaml",
		"juju-backup/dump/models/sub/x.yaml",
	} {
		c.Logf("path %q", name)
		files := validFiles()
		files[name] = []byte("payload: {}\n")
		path, _ := writeArchive(c, files)

		_, err := recovery.ValidateArchive(c.Context(), path, "")
		c.Check(err, tc.ErrorMatches, "unexpected model dump path .*",
			tc.Commentf("path %q", name))
	}
}

func (s *validateSuite) TestValidateArchiveMetadataMissingAgentVersion(c *tc.C) {
	files := validFiles()
	files["juju-backup/metadata.json"] = []byte(strings.Replace(
		metadataJSON("4.1.0"), `"Version":"4.1.0",`, ``, 1))
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches,
		"juju-backup/metadata.json does not record the source agent version")
}

func (s *validateSuite) TestValidateArchiveMetadataMalformedJSON(c *tc.C) {
	files := validFiles()
	files["juju-backup/metadata.json"] = []byte("{")
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches, "parsing juju-backup/metadata.json: .*")
}

func (s *validateSuite) TestValidateArchiveControllerDumpMalformedYAML(c *tc.C) {
	files := validFiles()
	files["juju-backup/dump/controller.yaml"] = []byte("payload: {\n")
	path, sum := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorMatches, "parsing juju-backup/dump/controller.yaml: .*")
}

// A manifest entry whose kind recovery does not recognise is a
// component recovery cannot account for; the writer never emits one —
// its classifier fails on unknown paths instead.
func (s *validateSuite) TestValidateArchiveManifestRejectsUnknownKind(c *tc.C) {
	files := validFiles()
	// The entry records the correct digest, so only the unrecognised
	// kind fails the archive.
	rootTar := files["juju-backup/root.tar"]
	sum := sha256.Sum256(rootTar)
	manifest := corebackups.NewManifest([]corebackups.ManifestEntry{{
		Path:   "juju-backup/root.tar",
		Kind:   "dump",
		Size:   int64(len(rootTar)),
		SHA256: hex.EncodeToString(sum[:]),
	}})
	reader, err := manifest.AsJSONBuffer()
	c.Assert(err, tc.ErrorIsNil)
	manifestJSON, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	files["juju-backup/manifest.json"] = manifestJSON
	path, _ := writeArchive(c, files)

	_, err = recovery.ValidateArchive(c.Context(), path, "")
	c.Assert(err, tc.ErrorMatches,
		`juju-backup/manifest.json lists "juju-backup/root.tar" with unrecognised kind "dump"`)
}

func (s *validateSuite) TestValidateArchiveWithManifest(c *tc.C) {
	files := withManifest(c, validFiles())
	path, sum := writeArchive(c, files)

	info, err := recovery.ValidateArchive(c.Context(), path, sum)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.ControllerUUID, tc.Equals, testControllerUUID)
	c.Check(info.Checksum, tc.Equals, sum)
}

// A component modified after the manifest was written disagrees with
// the manifest's recorded size and hash: the archive must be rejected
// even though its outer checksum (computed over the corrupt bytes) is
// not consulted.
func (s *validateSuite) TestValidateArchiveManifestDetectsTamperedDump(c *tc.C) {
	files := withManifest(c, validFiles())
	files["juju-backup/dump/models/"+testModelAUUID+".yaml"] =
		[]byte("payload: {tampered: true}\n")
	path, _ := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, "")
	c.Assert(err, tc.ErrorMatches,
		"juju-backup/manifest.json records size .* but the archive entry has size .*")
}

// A manifest entry for a file the archive does not hold means the
// archive lost a component after creation.
func (s *validateSuite) TestValidateArchiveManifestListsMissingEntry(c *tc.C) {
	files := withManifest(c, validFiles())
	delete(files, "juju-backup/root.tar")
	path, _ := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, "")
	c.Assert(err, tc.ErrorMatches,
		"juju-backup/manifest.json lists \"juju-backup/root.tar\", which is not in the archive")
}

// An archive entry the manifest does not list was added after
// creation: the manifest indexes the archive, never vice versa.
func (s *validateSuite) TestValidateArchiveManifestOmitsEntry(c *tc.C) {
	files := withManifest(c, validFiles())
	files["juju-backup/extra.bin"] = []byte("x")
	path, _ := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, "")
	c.Assert(err, tc.ErrorMatches,
		"archive entry \"juju-backup/extra.bin\" is not listed in juju-backup/manifest.json")
}

// A model dump entry whose model UUID does not match its path is an
// internally inconsistent manifest.
func (s *validateSuite) TestValidateArchiveManifestModelUUIDMismatch(c *tc.C) {
	files := withManifest(c, validFiles())
	manifest, err := corebackups.NewManifestJSONReader(
		bytes.NewReader(files["juju-backup/manifest.json"]))
	c.Assert(err, tc.ErrorIsNil)
	for i, f := range manifest.Files {
		if f.Kind == corebackups.ManifestKindModelDump {
			manifest.Files[i].ModelUUID = testModelBUUID
			break
		}
	}
	reader, err := manifest.AsJSONBuffer()
	c.Assert(err, tc.ErrorIsNil)
	manifestJSON, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	files["juju-backup/manifest.json"] = manifestJSON
	path, _ := writeArchive(c, files)

	_, err = recovery.ValidateArchive(c.Context(), path, "")
	c.Assert(err, tc.ErrorMatches,
		"juju-backup/manifest.json lists model dump .* with mismatched model UUID .*")
}

func (s *validateSuite) TestValidateArchiveManifestInvalidJSON(c *tc.C) {
	files := validFiles()
	files["juju-backup/manifest.json"] = []byte("{")
	path, _ := writeArchive(c, files)

	_, err := recovery.ValidateArchive(c.Context(), path, "")
	c.Assert(err, tc.ErrorMatches, "parsing juju-backup/manifest.json: .*")
}

func (s *validateSuite) TestValidateArchiveManifestDuplicateListing(c *tc.C) {
	files := validFiles()
	// Both entries record the correct digest, so only the duplicate
	// listing itself fails the archive.
	metadata := files["juju-backup/metadata.json"]
	sum := sha256.Sum256(metadata)
	entry := corebackups.ManifestEntry{
		Path:   "juju-backup/metadata.json",
		Kind:   corebackups.ManifestKindMetadata,
		Size:   int64(len(metadata)),
		SHA256: hex.EncodeToString(sum[:]),
	}
	manifest := corebackups.NewManifest([]corebackups.ManifestEntry{entry, entry})
	reader, err := manifest.AsJSONBuffer()
	c.Assert(err, tc.ErrorIsNil)
	manifestJSON, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	files["juju-backup/manifest.json"] = manifestJSON
	path, _ := writeArchive(c, files)

	_, err = recovery.ValidateArchive(c.Context(), path, "")
	c.Assert(err, tc.ErrorMatches,
		"juju-backup/manifest.json lists \"juju-backup/metadata.json\" twice")
}
