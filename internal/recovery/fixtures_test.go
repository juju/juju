// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/tc"

	corebackups "github.com/juju/juju/core/backups"
	domainrecovery "github.com/juju/juju/domain/recovery"
)

const (
	// testControllerUUID is the fixture source controller's identity.
	testControllerUUID = "c0ffee00-1111-2222-3333-444455556666"

	// testControllerModelUUID is the fixture controller model's identity.
	testControllerModelUUID = "c0ffee00-aaaa-2222-3333-444455556666"

	// testModelAUUID and testModelBUUID are fixture workload models.
	testModelAUUID = "c0ffee00-bbbb-2222-3333-444455556666"
	testModelBUUID = "c0ffee00-cccc-2222-3333-444455556666"
)

// metadataJSON renders a backup metadata manifest for the given agent
// version.
func metadataJSON(agentVersion string) string {
	return `{` +
		`"ID":"20260930-120000.aabb-ccdd-eeff",` +
		`"FormatVersion":2,` +
		`"Checksum":"abc",` +
		`"ChecksumFormat":"SHA-256, hex encoded",` +
		`"Size":10,` +
		`"Stored":"0001-01-01T00:00:00Z",` +
		`"Started":"2026-09-30T12:00:00Z",` +
		`"Finished":"2026-09-30T12:00:34Z",` +
		`"Notes":"",` +
		`"ModelUUID":"` + testControllerModelUUID + `",` +
		`"Machine":"0",` +
		`"Hostname":"myhost",` +
		`"Version":"` + agentVersion + `",` +
		`"ControllerUUID":"` + testControllerUUID + `",` +
		`"HANodes":3,` +
		`"ControllerMachineID":"0",` +
		`"ControllerMachineInstanceID":"inst-10101010"` +
		`}` + "\n"
}

// controllerDump renders a minimal controller database dump with the
// given per-model cloud type IDs and model type IDs. Cloud type 1 is
// "lxd", 2 is "kubernetes"; model type 0 is "iaas", 1 is "caas".
func controllerDump(models ...[3]string) string {
	var out strings.Builder
	out.WriteString("version: 4.1.0\n" +
		"payload:\n" +
		"  controller:\n" +
		"  - uuid: " + testControllerUUID + "\n" +
		"    model_uuid: " + testControllerModelUUID + "\n" +
		"    target_version: 4.1.0\n" +
		"    api_port: \"17070\"\n" +
		"    ca_cert: source-ca-cert\n" +
		"    ca_private_key: source-ca-key\n" +
		"  cloud:\n" +
		"  - uuid: cloud-lxd\n    name: lxd\n    cloud_type_id: 1\n    endpoint: ''\n    skip_tls_verify: false\n" +
		"  - uuid: cloud-k8s\n    name: myk8s\n    cloud_type_id: 2\n    endpoint: ''\n    skip_tls_verify: false\n" +
		"  cloud_type:\n" +
		"  - id: 1\n    type: lxd\n" +
		"  - id: 2\n    type: kubernetes\n" +
		"  model_type:\n" +
		"  - id: 0\n    type: iaas\n" +
		"  - id: 1\n    type: caas\n" +
		"  controller_config:\n" +
		"  - key: controller-name\n    value: source-ctrl\n" +
		"  model:\n")
	for _, m := range models {
		fmt.Fprintf(&out,
			"  - uuid: %s\n    name: %s\n    cloud_uuid: %s\n    model_type_id: %s\n"+
				"    activated: true\n    life_id: 0\n    qualifier: ''\n",
			m[0], m[1], m[2], "0")
	}
	return out.String()
}

// caasModelDump renders a minimal CAAS model database dump with the
// given applications. Each application is [uuid, name]; units and
// storage are attached deterministically: unit <name>/0 per application,
// and one alive filesystem per unit whose provider_id is the PVC name
// "<app>-<uuid[:6]>-0".
func caasModelDump(apps ...[2]string) string {
	var out strings.Builder
	out.WriteString("version: 4.1.0\npayload:\n  net_node:\n")
	for _, app := range apps {
		out.WriteString("  - uuid: node-" + app[1] + "\n")
	}
	out.WriteString("  application:\n")
	for _, app := range apps {
		fmt.Fprintf(&out,
			"  - uuid: %s\n    name: %s\n    life_id: 0\n    charm_uuid: charm-%s\n    password: ''\n",
			app[0], app[1], app[1])
	}
	out.WriteString("  unit:\n")
	for _, app := range apps {
		fmt.Fprintf(&out,
			"  - uuid: unit-%s\n    name: %s/0\n    life_id: 0\n    application_uuid: %s\n    net_node_uuid: node-%s\n",
			app[1], app[1], app[0], app[1])
	}
	out.WriteString("  storage_filesystem:\n")
	for _, app := range apps {
		fmt.Fprintf(&out,
			"  - uuid: fs-%s\n    filesystem_id: %s-vol\n    life_id: 0\n    provider_id: %s-%s-0\n",
			app[1], app[1], app[1], app[0][:6])
	}
	out.WriteString("  storage_filesystem_attachment:\n")
	for _, app := range apps {
		fmt.Fprintf(&out,
			"  - uuid: att-%s\n    storage_filesystem_uuid: fs-%s\n    net_node_uuid: node-%s\n    life_id: 0\n",
			app[1], app[1], app[1])
	}
	return out.String()
}

// modelRow is a helper building controllerDump model entries:
// uuid, name, cloud uuid. Model type is always iaas (0); use
// controllerDump directly for caas models.
func modelRow(uuid, name, cloudUUID string) [3]string {
	return [3]string{uuid, name, cloudUUID}
}

// rawControllerDump renders a controller database dump from raw YAML
// table rows. The lookup tables are fixed: cloud type 1 is "lxd", 2 is
// "kubernetes"; model type 0 is "iaas", 1 is "caas".
func rawControllerDump(controllerRows, cloudRows, modelRows string) string {
	var out strings.Builder
	out.WriteString("version: 4.1.0\npayload:\n")
	if controllerRows != "" {
		out.WriteString("  controller:\n" + controllerRows)
	}
	if cloudRows != "" {
		out.WriteString("  cloud:\n" + cloudRows)
	}
	out.WriteString("  cloud_type:\n" +
		"  - id: 1\n    type: lxd\n" +
		"  - id: 2\n    type: kubernetes\n" +
		"  model_type:\n" +
		"  - id: 0\n    type: iaas\n" +
		"  - id: 1\n    type: caas\n")
	if modelRows != "" {
		out.WriteString("  model:\n" + modelRows)
	}
	return out.String()
}

// validControllerRow renders the standard controller table row with CA
// material, matching the identities metadataJSON records.
func validControllerRow() string {
	return "  - uuid: " + testControllerUUID + "\n" +
		"    model_uuid: " + testControllerModelUUID + "\n" +
		"    ca_cert: source-ca-cert\n" +
		"    ca_private_key: source-ca-key\n"
}

// validCloudRows renders the standard cloud table: lxd and kubernetes.
func validCloudRows() string {
	return "  - uuid: cloud-lxd\n    name: lxd\n    cloud_type_id: 1\n" +
		"  - uuid: cloud-k8s\n    name: myk8s\n    cloud_type_id: 2\n"
}

// rawModelRow renders one model table row with an explicit model type.
func rawModelRow(uuid, name, cloudUUID, modelTypeID string) string {
	return "  - uuid: " + uuid + "\n" +
		"    name: " + name + "\n" +
		"    cloud_uuid: " + cloudUUID + "\n" +
		"    model_type_id: " + modelTypeID + "\n"
}

// dumpOnlyFiles returns a minimal archive file set with the given
// controller dump: enough to reach buildArchiveInfo, whose rejection
// branches fire before the model-dump inventory cross-check.
func dumpOnlyFiles(controllerYAML string) map[string][]byte {
	return map[string][]byte{
		"juju-backup/metadata.json":        []byte(metadataJSON("4.1.0")),
		"juju-backup/dump/controller.yaml": []byte(controllerYAML),
		"juju-backup/root.tar":             []byte("blobs"),
	}
}

// caasFiles returns a valid archive file set whose workload-a model is
// CAAS on the kubernetes cloud and carries the given model dump.
func caasFiles(modelDump string) map[string][]byte {
	files := validFiles()
	// Turn workload-a into a CAAS model on the kubernetes cloud:
	// controllerDump hardcodes model_type_id 0, patch the one workload
	// row to caas (1).
	files["juju-backup/dump/controller.yaml"] = []byte(strings.Replace(
		controllerDump(
			modelRow(testControllerModelUUID, "controller", "cloud-lxd"),
			modelRow(testModelAUUID, "workload-a", "cloud-k8s"),
		),
		"uuid: "+testModelAUUID+"\n    name: workload-a\n    cloud_uuid: cloud-k8s\n    model_type_id: 0",
		"uuid: "+testModelAUUID+"\n    name: workload-a\n    cloud_uuid: cloud-k8s\n    model_type_id: 1", 1))
	// workload-b is gone from the controller dump; its dump file must go
	// too or the inventory cross-check fails.
	delete(files, "juju-backup/dump/models/"+testModelBUUID+".yaml")
	files["juju-backup/dump/models/"+testModelAUUID+".yaml"] = []byte(modelDump)
	return files
}

// findModel returns the archive summary's entry for modelUUID.
func findModel(c *tc.C, info *domainrecovery.ArchiveInfo, modelUUID string) *domainrecovery.ModelInfo {
	for i := range info.Models {
		if info.Models[i].UUID == modelUUID {
			return &info.Models[i]
		}
	}
	c.Fatalf("model %q not in summary", modelUUID)
	return nil
}

// makeArchive renders a gzipped tar archive from the named files.
func makeArchive(c *tc.C, files map[string][]byte) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := files[name]
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o600,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}
		c.Assert(tw.WriteHeader(hdr), tc.ErrorIsNil)
		_, err := tw.Write(data)
		c.Assert(err, tc.ErrorIsNil)
	}
	c.Assert(tw.Close(), tc.ErrorIsNil)
	c.Assert(gz.Close(), tc.ErrorIsNil)
	return buf.Bytes()
}

// writeArchive writes the archive to a fresh temp file and returns its
// path and SHA-256 checksum (hex).
func writeArchive(c *tc.C, files map[string][]byte) (string, string) {
	archive := makeArchive(c, files)
	sum := sha256.Sum256(archive)
	path := filepath.Join(c.MkDir(), "juju-backup.tar.gz")
	c.Assert(os.WriteFile(path, archive, 0o600), tc.ErrorIsNil)
	return path, hex.EncodeToString(sum[:])
}

// validFiles returns a complete, valid recovery archive file set.
func validFiles() map[string][]byte {
	return map[string][]byte{
		"juju-backup/metadata.json": []byte(metadataJSON("4.1.0")),
		"juju-backup/dump/controller.yaml": []byte(controllerDump(
			modelRow(testControllerModelUUID, "controller", "cloud-lxd"),
			modelRow(testModelAUUID, "workload-a", "cloud-lxd"),
			modelRow(testModelBUUID, "workload-b", "cloud-lxd"),
		)),
		"juju-backup/dump/models/" + testControllerModelUUID + ".yaml": []byte("payload: {}\n"),
		"juju-backup/dump/models/" + testModelAUUID + ".yaml":          []byte("payload: {}\n"),
		"juju-backup/dump/models/" + testModelBUUID + ".yaml":          []byte("payload: {}\n"),
		"juju-backup/root.tar": []byte("blobs"),
	}
}

// withManifest adds a correct content manifest for the given archive
// file set, mirroring what backup creation writes: one entry per file
// with its size and SHA-256 content hash.
func withManifest(c *tc.C, files map[string][]byte) map[string][]byte {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]corebackups.ManifestEntry, 0, len(names))
	for _, name := range names {
		data := files[name]
		sum := sha256.Sum256(data)
		kind, modelUUID, err := corebackups.ClassifyManifestPath(name)
		c.Assert(err, tc.ErrorIsNil)
		entries = append(entries, corebackups.ManifestEntry{
			Path:      name,
			Kind:      kind,
			Size:      int64(len(data)),
			SHA256:    hex.EncodeToString(sum[:]),
			ModelUUID: modelUUID,
		})
	}
	reader, err := corebackups.NewManifest(entries).AsJSONBuffer()
	c.Assert(err, tc.ErrorIsNil)
	data, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	files["juju-backup/manifest.json"] = data
	return files
}
