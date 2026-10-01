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
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/tc"
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
