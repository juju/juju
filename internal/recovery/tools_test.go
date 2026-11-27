// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/recovery"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
)

type archivedToolsSuite struct {
	coretesting.BaseSuite
}

func TestArchivedToolsSuite(t *testing.T) {
	tc.Run(t, &archivedToolsSuite{})
}

type archivedToolFile struct {
	name     string
	contents []byte
	mode     int64
	typeflag byte
}

func makeToolsTar(c *tc.C, files []archivedToolFile, compressed bool) []byte {
	var buf bytes.Buffer
	var writer io.Writer = &buf
	var gz *gzip.Writer
	if compressed {
		gz = gzip.NewWriter(writer)
		writer = gz
	}
	tw := tar.NewWriter(writer)
	for _, file := range files {
		typeflag := file.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		header := &tar.Header{
			Name: file.name, Mode: file.mode, Typeflag: typeflag,
			Size: int64(len(file.contents)),
		}
		c.Assert(tw.WriteHeader(header), tc.ErrorIsNil)
		_, err := tw.Write(file.contents)
		c.Assert(err, tc.ErrorIsNil)
	}
	c.Assert(tw.Close(), tc.ErrorIsNil)
	if gz != nil {
		c.Assert(gz.Close(), tc.ErrorIsNil)
	}
	return buf.Bytes()
}

func (s *archivedToolsSuite) stageArchive(c *tc.C, files []archivedToolFile) (string, string) {
	root := makeToolsTar(c, files, false)
	return writeArchive(c, map[string][]byte{"juju-backup/root.tar": root})
}

func (s *archivedToolsSuite) TestStageInstalledAgentForDifferentArchitecture(c *tc.C) {
	version := semversion.MustParseBinary("4.1.0-ubuntu-arm64")
	dir := "var/lib/juju/tools/" + version.String()
	metadata, err := json.Marshal(&tools.Tools{Version: version})
	c.Assert(err, tc.ErrorIsNil)
	archive, checksum := s.stageArchive(c, []archivedToolFile{
		{name: path.Join(dir, "jujuagentd"), contents: []byte("archived agent"), mode: 0755},
		{name: path.Join(dir, "FORCE-VERSION"), contents: []byte("4.1.0"), mode: 0644},
		{name: path.Join(dir, "downloaded-tools.txt"), contents: metadata, mode: 0644},
	})

	staged, err := recovery.StageAgentTools(c.Context(), archive, checksum, version)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(staged, tc.NotNil)
	filename := strings.TrimPrefix(staged.Tools.URL, "file://")
	data, err := os.ReadFile(filename)
	c.Assert(err, tc.ErrorIsNil)
	sum := sha256.Sum256(data)
	c.Check(staged.Tools.Version, tc.Equals, version)
	c.Check(staged.Tools.SHA256, tc.Equals, hex.EncodeToString(sum[:]))
	c.Check(staged.Tools.Size, tc.Equals, int64(len(data)))
	second, err := recovery.StageAgentTools(c.Context(), archive, checksum, version)
	c.Assert(err, tc.ErrorIsNil)
	secondData, err := os.ReadFile(strings.TrimPrefix(second.Tools.URL, "file://"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(secondData, tc.DeepEquals, data)
	c.Check(second.Tools.SHA256, tc.Equals, staged.Tools.SHA256)
	c.Assert(second.Close(), tc.ErrorIsNil)

	gz, err := gzip.NewReader(bytes.NewReader(data))
	c.Assert(err, tc.ErrorIsNil)
	defer gz.Close()
	tr := tar.NewReader(gz)
	got := make(map[string]string)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		c.Assert(err, tc.ErrorIsNil)
		contents, err := io.ReadAll(tr)
		c.Assert(err, tc.ErrorIsNil)
		got[header.Name] = string(contents)
		if header.Name == "jujuagentd" {
			c.Check(header.Mode&0111 != 0, tc.IsTrue)
		}
	}
	c.Check(got, tc.DeepEquals, map[string]string{
		"jujuagentd":    "archived agent",
		"FORCE-VERSION": "4.1.0",
	})
	c.Assert(staged.Close(), tc.ErrorIsNil)
	_, err = os.Stat(filename)
	c.Check(os.IsNotExist(err), tc.IsTrue)
}

func (s *archivedToolsSuite) TestRetainsOriginalAgentBundle(c *tc.C) {
	version := semversion.MustParseBinary("4.1.0-ubuntu-amd64")
	dir := "var/lib/juju/tools/" + version.String()
	original := makeToolsTar(c, []archivedToolFile{
		{name: "jujuagentd", contents: []byte("original agent"), mode: 0755},
	}, true)
	sum := sha256.Sum256(original)
	digest := hex.EncodeToString(sum[:])
	metadata, err := json.Marshal(&tools.Tools{Version: version, SHA256: digest})
	c.Assert(err, tc.ErrorIsNil)
	archive, checksum := s.stageArchive(c, []archivedToolFile{
		{name: path.Join(dir, "tools.tar.gz"), contents: original, mode: 0644},
		{name: path.Join(dir, "downloaded-tools.txt"), contents: metadata, mode: 0644},
	})

	staged, err := recovery.StageAgentTools(c.Context(), archive, checksum, version)
	c.Assert(err, tc.ErrorIsNil)
	defer staged.Close()
	contents, err := os.ReadFile(strings.TrimPrefix(staged.Tools.URL, "file://"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(contents, tc.DeepEquals, original)
	c.Check(staged.Tools.SHA256, tc.Equals, digest)
}

func (s *archivedToolsSuite) TestRetainsAgentBundleFromControllerObjectStore(c *tc.C) {
	version := semversion.MustParseBinary("4.1.0-ubuntu-amd64")
	dir := "var/lib/juju/tools/" + version.String()
	original := makeToolsTar(c, []archivedToolFile{
		{name: "jujuagentd", contents: []byte("installed agent"), mode: 0755},
	}, true)
	sum256, sum384 := sha256.Sum256(original), sha512.Sum384(original)
	digest := hex.EncodeToString(sum256[:])
	metadata, err := json.Marshal(&tools.Tools{Version: version, SHA256: digest, Size: int64(len(original))})
	c.Assert(err, tc.ErrorIsNil)
	installed := []archivedToolFile{
		{name: path.Join(dir, "jujuagentd"), contents: []byte("installed agent"), mode: 0755},
		{name: path.Join(dir, "downloaded-tools.txt"), contents: metadata, mode: 0644},
	}
	object := archivedToolFile{
		name:     "var/lib/juju/objectstore/controller/" + hex.EncodeToString(sum384[:]),
		contents: original,
		mode:     0640,
	}
	// Both the backup writer's normal order and the reverse order must work.
	for _, files := range [][]archivedToolFile{
		append(append([]archivedToolFile{}, installed...), object),
		append([]archivedToolFile{object}, installed...),
	} {
		archive, checksum := s.stageArchive(c, files)
		staged, err := recovery.StageAgentTools(c.Context(), archive, checksum, version)
		c.Assert(err, tc.ErrorIsNil)
		contents, err := os.ReadFile(strings.TrimPrefix(staged.Tools.URL, "file://"))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(contents, tc.DeepEquals, original)
		c.Check(staged.Tools.SHA256, tc.Equals, digest)
		c.Assert(staged.Close(), tc.ErrorIsNil)
	}
}

// TestStageToolsFromCreatedBackup covers the actual backup writer, including
// a non-default data directory and its leading-slash handling.
func (s *archivedToolsSuite) TestStageToolsFromCreatedBackup(c *tc.C) {
	version := semversion.MustParseBinary("4.1.0-ubuntu-amd64")
	dataDir := filepath.Join(c.MkDir(), "custom-data")
	dir := filepath.Join(dataDir, "tools", version.String())
	c.Assert(os.MkdirAll(dir, 0700), tc.ErrorIsNil)
	agentPath := filepath.Join(dir, "jujuagentd")
	c.Assert(os.WriteFile(agentPath, []byte("agent from backup"), 0755), tc.ErrorIsNil)
	original := makeToolsTar(c, []archivedToolFile{
		{name: "jujuagentd", contents: []byte("agent from backup"), mode: 0755},
	}, true)
	sum256, sum384 := sha256.Sum256(original), sha512.Sum384(original)
	digest := hex.EncodeToString(sum256[:])
	toolsMetadata, err := json.Marshal(&tools.Tools{Version: version, SHA256: digest, Size: int64(len(original))})
	c.Assert(err, tc.ErrorIsNil)
	metadataPath := filepath.Join(dir, "downloaded-tools.txt")
	c.Assert(os.WriteFile(metadataPath, toolsMetadata, 0600), tc.ErrorIsNil)
	metadata := corebackups.NewMetadata(time.Now())
	filename, err := corebackups.Create(c.Context(), metadata, corebackups.CreateArgs{
		DestinationDir: c.MkDir(),
		Clock:          clock.WallClock,
		DataDir:        dataDir,
		FilesToBackUp:  []string{agentPath, metadataPath},
		ObjectEntries: []corebackups.ObjectEntry{{
			Namespace: "controller", SHA256: digest, SHA384: hex.EncodeToString(sum384[:]),
			Size: int64(len(original)),
			Source: func(context.Context) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(original)), nil
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	staged, err := recovery.StageAgentTools(c.Context(), filename, metadata.Checksum(), version)
	c.Assert(err, tc.ErrorIsNil)
	defer staged.Close()
	c.Check(staged.Tools.Version, tc.Equals, version)
	c.Check(staged.Tools.SHA256, tc.Equals, digest)
	c.Check(staged.Tools.Size, tc.Equals, int64(len(original)))
	bundle, err := os.Open(strings.TrimPrefix(staged.Tools.URL, "file://"))
	c.Assert(err, tc.ErrorIsNil)
	defer bundle.Close()
	gz, err := gzip.NewReader(bundle)
	c.Assert(err, tc.ErrorIsNil)
	defer gz.Close()
	tr := tar.NewReader(gz)
	header, err := tr.Next()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(header.Name, tc.Equals, "jujuagentd")
	contents, err := io.ReadAll(tr)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Equals, "agent from backup")
}

func (s *archivedToolsSuite) TestRejectsInvalidArchivedTools(c *tc.C) {
	version := semversion.MustParseBinary("4.1.0-ubuntu-amd64")
	dir := "var/lib/juju/tools/" + version.String()
	wrongMetadata, err := json.Marshal(&tools.Tools{
		Version: semversion.MustParseBinary("4.1.0-ubuntu-arm64"),
	})
	c.Assert(err, tc.ErrorIsNil)
	wrongDigest, err := json.Marshal(&tools.Tools{
		Version: version, SHA256: strings.Repeat("0", 64),
	})
	c.Assert(err, tc.ErrorIsNil)
	badBundle := makeToolsTar(c, []archivedToolFile{
		{name: "../jujuagentd", contents: []byte("bad"), mode: 0755},
	}, true)
	goodBundle := makeToolsTar(c, []archivedToolFile{
		{name: "jujuagentd", contents: []byte("agent"), mode: 0755},
	}, true)
	goodDigest := sha256.Sum256(goodBundle)
	goodMetadata, err := json.Marshal(&tools.Tools{
		Version: version, SHA256: hex.EncodeToString(goodDigest[:]), Size: int64(len(goodBundle)),
	})
	c.Assert(err, tc.ErrorIsNil)
	wrongSize, err := json.Marshal(&tools.Tools{
		Version: version, SHA256: hex.EncodeToString(goodDigest[:]), Size: int64(len(goodBundle) + 1),
	})
	c.Assert(err, tc.ErrorIsNil)
	wrongVersionBundle := makeToolsTar(c, []archivedToolFile{
		{name: "jujuagentd", contents: []byte("agent"), mode: 0755},
		{name: "FORCE-VERSION", contents: []byte("4.0.0"), mode: 0644},
	}, true)
	tests := []struct {
		name  string
		files []archivedToolFile
		match string
	}{
		{"missing", nil, `.*does not contain agent.*`},
		{"wrong version", []archivedToolFile{{name: "var/lib/juju/tools/4.0.0-ubuntu-amd64/jujuagentd", contents: []byte("bad"), mode: 0755}}, `.*does not contain agent.*`},
		{"non-executable", []archivedToolFile{{name: path.Join(dir, "jujuagentd"), contents: []byte("bad"), mode: 0644}}, `archived agent binary is not executable`},
		{"symlink", []archivedToolFile{{name: path.Join(dir, "jujuagentd"), typeflag: tar.TypeSymlink}}, `archived agent file .* is not regular`},
		{"duplicate", []archivedToolFile{{name: path.Join(dir, "jujuagentd"), contents: []byte("a"), mode: 0755}, {name: path.Join(dir, "jujuagentd"), contents: []byte("b"), mode: 0755}}, `duplicate archived agent file.*`},
		{"unsafe path", []archivedToolFile{{name: "/" + path.Join(dir, "jujuagentd"), contents: []byte("bad"), mode: 0755}}, `unsafe archived agent path.*`},
		{"traversal", []archivedToolFile{{name: "var/lib/juju/tools/other/../" + version.String() + "/jujuagentd", contents: []byte("bad"), mode: 0755}}, `unsafe archived agent path.*`},
		{"multiple directories", []archivedToolFile{{name: path.Join(dir, "jujuagentd"), contents: []byte("a"), mode: 0755}, {name: path.Join("other/tools", version.String(), "jujuagentd"), contents: []byte("b"), mode: 0755}}, `multiple archived agent directories.*`},
		{"metadata mismatch", []archivedToolFile{{name: path.Join(dir, "jujuagentd"), contents: []byte("a"), mode: 0755}, {name: path.Join(dir, "downloaded-tools.txt"), contents: wrongMetadata, mode: 0644}}, `archived agent metadata version.*does not match.*`},
		{"metadata trailing data", []archivedToolFile{{name: path.Join(dir, "jujuagentd"), contents: []byte("a"), mode: 0755}, {name: path.Join(dir, "downloaded-tools.txt"), contents: append(append([]byte{}, goodMetadata...), []byte("garbage")...), mode: 0644}}, `reading archived agent metadata: .*`},
		{"unsafe bundle", []archivedToolFile{{name: path.Join(dir, "tools.tar.gz"), contents: badBundle, mode: 0644}}, `invalid archived agent bundle: unsafe agent bundle entry.*`},
		{"bundle checksum", []archivedToolFile{{name: path.Join(dir, "tools.tar.gz"), contents: goodBundle, mode: 0644}, {name: path.Join(dir, "downloaded-tools.txt"), contents: wrongDigest, mode: 0644}}, `archived agent bundle checksum disagrees with its metadata`},
		{"bundle size", []archivedToolFile{{name: path.Join(dir, "tools.tar.gz"), contents: goodBundle, mode: 0644}, {name: path.Join(dir, "downloaded-tools.txt"), contents: wrongSize, mode: 0644}}, `archived agent bundle size disagrees with its metadata`},
		{"bundle forced version", []archivedToolFile{{name: path.Join(dir, "tools.tar.gz"), contents: wrongVersionBundle, mode: 0644}}, `invalid archived agent bundle: archived forced version .* does not match .*`},
		{"bundle differs from installed", []archivedToolFile{{name: path.Join(dir, "tools.tar.gz"), contents: goodBundle, mode: 0644}, {name: path.Join(dir, "jujuagentd"), contents: []byte("different"), mode: 0755}}, `invalid archived agent bundle: archived agent bundle differs from the installed agent`},
		{"invalid object name", []archivedToolFile{{name: path.Join(dir, "downloaded-tools.txt"), contents: goodMetadata, mode: 0644}, {name: "var/lib/juju/objectstore/controller/" + strings.Repeat("0", 96), contents: goodBundle, mode: 0640}}, `archived agent object .* has an invalid SHA-384`},
		{"forced version", []archivedToolFile{{name: path.Join(dir, "jujuagentd"), contents: []byte("a"), mode: 0755}, {name: path.Join(dir, "FORCE-VERSION"), contents: []byte("4.0.0"), mode: 0644}}, `archived forced version .* does not match .*`},
	}
	for _, test := range tests {
		c.Logf("%s", test.name)
		archive, checksum := s.stageArchive(c, test.files)
		_, err := recovery.StageAgentTools(c.Context(), archive, checksum, version)
		c.Check(err, tc.ErrorMatches, test.match)
	}
}

func (s *archivedToolsSuite) TestRejectsChangedArchive(c *tc.C) {
	version := semversion.MustParseBinary("4.1.0-ubuntu-amd64")
	dir := "var/lib/juju/tools/" + version.String()
	archive, _ := s.stageArchive(c, []archivedToolFile{
		{name: path.Join(dir, "jujuagentd"), contents: []byte("agent"), mode: 0755},
	})
	workspace := c.MkDir()
	c.Setenv("TMPDIR", workspace)
	_, err := recovery.StageAgentTools(c.Context(), archive, strings.Repeat("0", 64), version)
	c.Check(err, tc.ErrorMatches, "recovery archive checksum does not match.*")
	remaining, err := os.ReadDir(workspace)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(remaining, tc.HasLen, 0)
}

func (s *archivedToolsSuite) TestAcceptsCleanableRootArchivePath(c *tc.C) {
	version := semversion.MustParseBinary("4.1.0-ubuntu-amd64")
	root := makeToolsTar(c, []archivedToolFile{{
		name:     "var/lib/juju/tools/" + version.String() + "/jujuagentd",
		contents: []byte("agent"), mode: 0755,
	}}, false)
	archive, checksum := writeArchive(c, map[string][]byte{
		"./juju-backup/root.tar": root,
	})
	staged, err := recovery.StageAgentTools(c.Context(), archive, checksum, version)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(staged.Close(), tc.ErrorIsNil)
}

func (s *archivedToolsSuite) TestRejectsDuplicateRootArchive(c *tc.C) {
	version := semversion.MustParseBinary("4.1.0-ubuntu-amd64")
	root := makeToolsTar(c, []archivedToolFile{{
		name:     "var/lib/juju/tools/" + version.String() + "/jujuagentd",
		contents: []byte("agent"), mode: 0755,
	}}, false)
	archive, checksum := writeArchive(c, map[string][]byte{
		"./juju-backup/root.tar": root,
		"juju-backup/root.tar":   root,
	})
	_, err := recovery.StageAgentTools(c.Context(), archive, checksum, version)
	c.Check(err, tc.ErrorMatches, `invalid or duplicate .* in recovery archive`)
}

func (s *archivedToolsSuite) TestCancellationRemovesStagedFiles(c *tc.C) {
	version := semversion.MustParseBinary("4.1.0-ubuntu-amd64")
	dir := "var/lib/juju/tools/" + version.String()
	archive, checksum := s.stageArchive(c, []archivedToolFile{
		{name: path.Join(dir, "jujuagentd"), contents: []byte("agent"), mode: 0755},
	})
	workspace := c.MkDir()
	c.Setenv("TMPDIR", workspace)
	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	_, err := recovery.StageAgentTools(ctx, archive, checksum, version)
	c.Check(err, tc.ErrorIs, context.Canceled)
	remaining, err := os.ReadDir(workspace)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(remaining, tc.HasLen, 0)
}
