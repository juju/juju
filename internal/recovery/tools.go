// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
	jujunames "github.com/juju/juju/juju/names"
)

const (
	// A single agent bundle or one of its files must fit this bound.
	// Unlike the backup itself, agent tools are expected to be small.
	maxAgentToolsSize  = 1 << 30
	maxAgentToolsFiles = 128
)

// StagedAgentTools owns a temporary agent bundle extracted from a backup.
// Close must be called after the tools are no longer needed.
type StagedAgentTools struct {
	Tools *coretools.Tools
	dir   string
}

// Close removes the staged bundle.
func (s *StagedAgentTools) Close() error {
	return errors.Capture(os.RemoveAll(s.dir))
}

// StageAgentTools finds the requested agent in the backup's root.tar and
// prepares a tools tarball for machine bootstrap. The whole backup is
// checked against expectedSHA256 before the staged bundle is returned.
//
// The backup stores installed files, not necessarily their original tarball.
// If an original tarball is present, it is preferred so its existing digest
// is retained. Otherwise the installed files are bundled anew without
// executing the archived binary or assuming that the client's architecture
// matches. The reconstructed bundle carries the same installed agent, but
// it is a different tarball, so its digest need not match the original's.
// That is expected and harmless: either bundle serves the archived agent.
func StageAgentTools(
	ctx context.Context, archivePath, expectedSHA256 string, version semversion.Binary,
) (_ *StagedAgentTools, err error) {
	dir, err := os.MkdirTemp("", "juju-recovery-agent-")
	if err != nil {
		return nil, errors.Errorf("creating recovery agent workspace: %w", err)
	}
	defer func() {
		if err != nil {
			if cleanupErr := os.RemoveAll(dir); cleanupErr != nil {
				err = stderrors.Join(err, errors.Errorf("cleaning up archived agent: %w", cleanupErr))
			}
		}
	}()

	filesDir := filepath.Join(dir, "files")
	if err := os.Mkdir(filesDir, 0700); err != nil {
		return nil, errors.Capture(err)
	}
	var files archivedAgentFiles
	if err := scanArchiveRoot(ctx, archivePath, expectedSHA256, func(reader io.Reader) error {
		var err error
		files, err = extractArchivedAgent(ctx, reader, filesDir, version)
		return err
	}); err != nil {
		return nil, errors.Capture(err)
	}
	if files.metadata != nil && files.metadata.Version != version {
		return nil, errors.Errorf("archived agent metadata version %q does not match %q",
			files.metadata.Version, version)
	}
	if files.metadata != nil && files.metadata.SHA256 != "" {
		digest := strings.TrimPrefix(files.metadata.SHA256, "sha256-")
		if len(digest) != sha256.Size*2 {
			return nil, errors.New("invalid archived agent checksum")
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return nil, errors.Errorf("invalid archived agent checksum: %w", err)
		}
	}
	// The backup writer normally puts the installed tools before its object
	// entries. If another writer chose the opposite order, scan once more
	// rather than silently replacing an available original bundle.
	if files.original == "" && files.object == "" && files.earlyObjects && usableAgentMetadata(files.metadata) {
		if err := scanArchiveRoot(ctx, archivePath, expectedSHA256, func(reader io.Reader) error {
			var err error
			files.object, err = findAgentObject(ctx, reader, dir, files.metadata)
			return err
		}); err != nil {
			return nil, errors.Capture(err)
		}
	}
	if files.original == "" {
		files.original = files.object
	}

	bundlePath := filepath.Join(dir, "agent.tar.gz")
	var size int64
	var digest string
	if files.original != "" {
		installedAgent := ""
		if files.agent {
			installedAgent = filepath.Join(filesDir, jujunames.JujuAgentd)
		}
		size, digest, err = inspectAgentBundle(ctx, files.original, installedAgent, version.Number)
		if err != nil {
			return nil, errors.Errorf("invalid archived agent bundle: %w", err)
		}
		if files.metadata != nil && files.metadata.SHA256 != "" &&
			!strings.EqualFold(digest, strings.TrimPrefix(files.metadata.SHA256, "sha256-")) {
			return nil, errors.New("archived agent bundle checksum disagrees with its metadata")
		}
		if files.metadata != nil && files.metadata.Size > 0 && files.metadata.Size != size {
			return nil, errors.New("archived agent bundle size disagrees with its metadata")
		}
		bundlePath = files.original
	} else {
		if !files.agent {
			return nil, errors.Errorf("recovery archive does not contain agent %q", version)
		}
		size, digest, err = bundleArchivedAgent(ctx, filesDir, bundlePath)
		if err != nil {
			return nil, errors.Errorf("bundling archived agent: %w", err)
		}
	}
	return &StagedAgentTools{
		Tools: &coretools.Tools{
			Version: version,
			URL:     "file://" + bundlePath,
			SHA256:  digest,
			Size:    size,
		},
		dir: dir,
	}, nil
}

// scanArchiveRoot streams root.tar to readRoot, and verifies the complete
// compressed backup before accepting any extracted data.
func scanArchiveRoot(ctx context.Context, archivePath, expectedSHA256 string, readRoot func(io.Reader) error) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return errors.Errorf("opening recovery archive: %w", err)
	}
	defer func() { _ = archive.Close() }()

	hasher := sha256.New()
	compressed := io.TeeReader(contextReader{ctx: ctx, reader: archive}, hasher)
	gz, err := gzip.NewReader(compressed)
	if err != nil {
		return errors.Errorf("reading recovery archive: %w", err)
	}
	defer func() { _ = gz.Close() }()

	var foundRoot bool
	outer := tar.NewReader(gz)
	for {
		header, err := outer.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.Errorf("reading recovery archive: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return errors.Capture(err)
		}
		if path.Clean(header.Name) != canonicalPaths.FilesArchive {
			continue
		}
		if foundRoot || (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA) {
			return errors.Errorf("invalid or duplicate %q in recovery archive", header.Name)
		}
		foundRoot = true
		if err := readRoot(outer); err != nil {
			return errors.Capture(err)
		}
	}
	// Consume the remainder to verify the gzip trailer and hash all bytes,
	// including any data following the tar end marker.
	if _, err := io.Copy(io.Discard, contextReader{ctx: ctx, reader: gz}); err != nil {
		return errors.Errorf("reading recovery archive: %w", err)
	}
	if err := gz.Close(); err != nil {
		return errors.Errorf("closing recovery archive: %w", err)
	}
	if _, err := io.Copy(io.Discard, compressed); err != nil {
		return errors.Errorf("reading recovery archive: %w", err)
	}
	checksum := corebackups.ChecksumSHA256(hex.EncodeToString(hasher.Sum(nil)))
	if expectedSHA256 == "" || !strings.EqualFold(checksum, corebackups.ChecksumSHA256(expectedSHA256)) {
		return errors.Errorf("recovery archive checksum does not match %q", expectedSHA256)
	}
	if !foundRoot {
		return errors.Errorf("recovery archive is missing %q", canonicalPaths.FilesArchive)
	}
	return nil
}

type archivedAgentFiles struct {
	agent        bool
	original     string
	object       string
	earlyObjects bool
	metadata     *coretools.Tools
}

// extractArchivedAgent reads only flat files from the matching versioned
// tools directory. No archive-provided path is used as a filesystem path.
func extractArchivedAgent(
	ctx context.Context, reader io.Reader, dir string, version semversion.Binary,
) (archivedAgentFiles, error) {
	var result archivedAgentFiles
	var sourceDir string
	var total int64
	seen := make(map[string]struct{})
	tr := tar.NewReader(contextReader{ctx: ctx, reader: reader})
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return result, nil
		}
		if err != nil {
			return result, errors.Errorf("reading archived agent files: %w", err)
		}
		name := path.Clean(header.Name)
		if isControllerObject(name) {
			if result.metadata == nil {
				result.earlyObjects = true
			} else if result.object == "" && result.original == "" {
				result.object, err = stageMatchingAgentObject(ctx, tr, header, filepath.Dir(dir), result.metadata)
				if err != nil {
					return result, errors.Capture(err)
				}
			}
			continue
		}
		parent := path.Dir(name)
		if path.Base(parent) != version.String() || path.Base(path.Dir(parent)) != "tools" {
			continue
		}
		if strings.HasPrefix(header.Name, "/") || strings.Contains(header.Name, "\\") ||
			strings.Contains("/"+header.Name+"/", "/../") {
			return result, errors.Errorf("unsafe archived agent path %q", header.Name)
		}
		if sourceDir == "" {
			sourceDir = parent
		} else if sourceDir != parent {
			return result, errors.Errorf("multiple archived agent directories for %q", version)
		}
		base := path.Base(name)
		if _, ok := seen[base]; ok {
			return result, errors.Errorf("duplicate archived agent file %q", base)
		}
		seen[base] = struct{}{}
		if len(seen) > maxAgentToolsFiles {
			return result, errors.New("too many archived agent files")
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			if base == jujunames.JujuAgentd || base == "tools.tar.gz" || base == "downloaded-tools.txt" {
				return result, errors.Errorf("archived agent file %q is not regular", base)
			}
			continue
		}
		if header.Size < 0 || header.Size > maxAgentToolsSize {
			return result, errors.Errorf("archived agent file %q exceeds size limit", base)
		}
		total += header.Size
		if total > maxAgentToolsSize {
			return result, errors.New("archived agent files exceed size limit")
		}
		if base == "downloaded-tools.txt" {
			if header.Size > maxMetadataSize {
				return result, errors.New("archived agent metadata exceeds size limit")
			}
			data, err := io.ReadAll(tr)
			if err != nil {
				return result, errors.Errorf("reading archived agent metadata: %w", err)
			}
			var metadata coretools.Tools
			if err := json.Unmarshal(data, &metadata); err != nil {
				return result, errors.Errorf("reading archived agent metadata: %w", err)
			}
			result.metadata = &metadata
			continue
		}
		if strings.HasSuffix(base, ".sha256") {
			continue
		}
		if base == jujunames.JujuAgentd {
			if header.Mode&0111 == 0 {
				return result, errors.New("archived agent binary is not executable")
			}
			result.agent = true
		}
		if base == "FORCE-VERSION" {
			if header.Size > maxMetadataSize {
				return result, errors.New("archived forced version exceeds size limit")
			}
			data, err := io.ReadAll(tr)
			if err != nil {
				return result, errors.Capture(err)
			}
			if strings.TrimSpace(string(data)) != version.Number.String() {
				return result, errors.Errorf("archived forced version %q does not match %q",
					strings.TrimSpace(string(data)), version.Number)
			}
			if err := os.WriteFile(filepath.Join(dir, base), data, os.FileMode(header.Mode)&0777); err != nil {
				return result, errors.Capture(err)
			}
			continue
		}
		target := filepath.Join(dir, base)
		if base == "tools.tar.gz" {
			result.original = target
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode)&0777)
		if err != nil {
			return result, errors.Capture(err)
		}
		_, copyErr := io.Copy(file, contextReader{ctx: ctx, reader: tr})
		closeErr := file.Close()
		if copyErr != nil {
			return result, errors.Capture(copyErr)
		}
		if closeErr != nil {
			return result, errors.Capture(closeErr)
		}
	}
}

func usableAgentMetadata(metadata *coretools.Tools) bool {
	if metadata == nil || metadata.Size <= 0 || metadata.Size > maxAgentToolsSize {
		return false
	}
	digest := strings.TrimPrefix(metadata.SHA256, "sha256-")
	if len(digest) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func isControllerObject(name string) bool {
	parent := path.Dir(name)
	return path.Base(parent) == "controller" &&
		path.Base(path.Dir(parent)) == "objectstore" && len(path.Base(name)) == sha512.Size384*2
}

// stageMatchingAgentObject copies only size-matching controller objects and
// retains one when its SHA-256 matches the archived agent metadata. The
// SHA-384 encoded in its object-store path must match too.
func stageMatchingAgentObject(
	ctx context.Context, reader io.Reader, header *tar.Header, dir string, metadata *coretools.Tools,
) (_ string, err error) {
	if !usableAgentMetadata(metadata) || header.Size != metadata.Size ||
		(header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA) {
		return "", nil
	}
	name := header.Name
	if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") ||
		strings.Contains("/"+name+"/", "/../") {
		return "", errors.Errorf("unsafe archived agent object path %q", name)
	}
	if _, err := hex.DecodeString(path.Base(name)); err != nil {
		return "", nil
	}
	file, err := os.CreateTemp(dir, "agent-object-")
	if err != nil {
		return "", errors.Capture(err)
	}
	filename := file.Name()
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			if cleanupErr := os.Remove(filename); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
				err = stderrors.Join(err, errors.Errorf("removing staged object: %w", cleanupErr))
			}
		}
	}()
	hash256, hash384 := sha256.New(), sha512.New384()
	if _, err := io.Copy(io.MultiWriter(file, hash256, hash384), contextReader{ctx: ctx, reader: reader}); err != nil {
		return "", errors.Capture(err)
	}
	if err := file.Close(); err != nil {
		return "", errors.Capture(err)
	}
	if !strings.EqualFold(hex.EncodeToString(hash256.Sum(nil)), strings.TrimPrefix(metadata.SHA256, "sha256-")) {
		return "", nil
	}
	if !strings.EqualFold(hex.EncodeToString(hash384.Sum(nil)), path.Base(name)) {
		return "", errors.Errorf("archived agent object %q has an invalid SHA-384", name)
	}
	keep = true
	return filename, nil
}

func findAgentObject(ctx context.Context, reader io.Reader, dir string, metadata *coretools.Tools) (string, error) {
	tr := tar.NewReader(contextReader{ctx: ctx, reader: reader})
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		if err != nil {
			return "", errors.Errorf("reading archived objects: %w", err)
		}
		if !isControllerObject(path.Clean(header.Name)) {
			continue
		}
		filename, err := stageMatchingAgentObject(ctx, tr, header, dir, metadata)
		if err != nil || filename != "" {
			return filename, errors.Capture(err)
		}
	}
}

// inspectAgentBundle rejects unsafe entries before a retained original bundle
// can be unpacked by machine setup.
func inspectAgentBundle(
	ctx context.Context, filename, installedAgent string, version semversion.Number,
) (int64, string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, "", errors.Capture(err)
	}
	defer func() { _ = file.Close() }()
	hasher := sha256.New()
	reader := io.TeeReader(contextReader{ctx: ctx, reader: file}, hasher)
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return 0, "", errors.Capture(err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	seen := make(map[string]struct{})
	var hasAgent bool
	var total int64
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, "", errors.Capture(err)
		}
		if header.Name == "" || strings.ContainsAny(header.Name, "/\\") ||
			header.Name == "." || header.Name == ".." ||
			(header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA) {
			return 0, "", errors.Errorf("unsafe agent bundle entry %q", header.Name)
		}
		if _, ok := seen[header.Name]; ok {
			return 0, "", errors.Errorf("duplicate agent bundle entry %q", header.Name)
		}
		seen[header.Name] = struct{}{}
		if len(seen) > maxAgentToolsFiles || header.Size < 0 || header.Size > maxAgentToolsSize-total {
			return 0, "", errors.New("archived agent bundle exceeds size limit")
		}
		total += header.Size
		if header.Name == jujunames.JujuAgentd {
			if header.Mode&0111 == 0 {
				return 0, "", errors.New("archived agent binary is not executable")
			}
			hasAgent = true
			if installedAgent != "" {
				if err := compareAgentFiles(ctx, tr, installedAgent); err != nil {
					return 0, "", errors.Capture(err)
				}
			}
		}
		if header.Name == "FORCE-VERSION" {
			if header.Size > maxMetadataSize {
				return 0, "", errors.New("archived forced version exceeds size limit")
			}
			data, err := io.ReadAll(tr)
			if err != nil {
				return 0, "", errors.Capture(err)
			}
			if strings.TrimSpace(string(data)) != version.String() {
				return 0, "", errors.Errorf("archived forced version %q does not match %q",
					strings.TrimSpace(string(data)), version)
			}
		}
	}
	if !hasAgent {
		return 0, "", errors.New("archived agent bundle has no jujuagentd")
	}
	if _, err := io.Copy(io.Discard, contextReader{ctx: ctx, reader: gz}); err != nil {
		return 0, "", errors.Capture(err)
	}
	if err := gz.Close(); err != nil {
		return 0, "", errors.Capture(err)
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return 0, "", errors.Capture(err)
	}
	stat, err := file.Stat()
	if err != nil {
		return 0, "", errors.Capture(err)
	}
	return stat.Size(), hex.EncodeToString(hasher.Sum(nil)), nil
}

func compareAgentFiles(ctx context.Context, bundled io.Reader, installedPath string) error {
	installed, err := os.Open(installedPath)
	if err != nil {
		return errors.Capture(err)
	}
	defer installed.Close()
	bundledHash, installedHash := sha256.New(), sha256.New()
	if _, err := io.Copy(bundledHash, contextReader{ctx: ctx, reader: bundled}); err != nil {
		return errors.Capture(err)
	}
	if _, err := io.Copy(installedHash, contextReader{ctx: ctx, reader: installed}); err != nil {
		return errors.Capture(err)
	}
	if !bytes.Equal(bundledHash.Sum(nil), installedHash.Sum(nil)) {
		return errors.New("archived agent bundle differs from the installed agent")
	}
	return nil
}

func bundleArchivedAgent(ctx context.Context, dir, filename string) (int64, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, "", errors.Capture(err)
	}
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return 0, "", errors.Capture(err)
	}
	defer func() { _ = file.Close() }()
	hasher := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(file, hasher))
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return 0, "", errors.Capture(err)
		}
		header := &tar.Header{Name: entry.Name(), Mode: int64(info.Mode().Perm()), Size: info.Size(), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(header); err != nil {
			return 0, "", errors.Capture(err)
		}
		input, err := os.Open(filepath.Join(dir, entry.Name()))
		if err != nil {
			return 0, "", errors.Capture(err)
		}
		_, copyErr := io.Copy(tw, contextReader{ctx: ctx, reader: input})
		closeErr := input.Close()
		if copyErr != nil {
			return 0, "", errors.Capture(copyErr)
		}
		if closeErr != nil {
			return 0, "", errors.Capture(closeErr)
		}
	}
	if err := tw.Close(); err != nil {
		return 0, "", errors.Capture(err)
	}
	if err := gz.Close(); err != nil {
		return 0, "", errors.Capture(err)
	}
	if err := file.Close(); err != nil {
		return 0, "", errors.Capture(err)
	}
	stat, err := os.Stat(filename)
	if err != nil {
		return 0, "", errors.Capture(err)
	}
	return stat.Size(), hex.EncodeToString(hasher.Sum(nil)), nil
}
