// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/juju/juju/internal/errors"
)

// DumpExportFunc marshals one database dump into writer. Implementations
// must stream (e.g. yaml.Encoder), so the dump's bytes never accumulate in
// memory.
type DumpExportFunc func(ctx context.Context, writer io.Writer) error

// NamedDump pairs a dump entry name with the function producing its
// contents, e.g. "controller.yaml" or "models/<uuid>.yaml".
type NamedDump struct {
	Name   string
	Export DumpExportFunc
}

// DumpStaging is a directory of staged database dumps. Its files feed
// directly into [Create] as [DumpEntry] readers. Close must be called once
// the dumps have been archived; it releases the readers and removes the
// staging directory, so a failed archive creation leaves no partial dumps
// behind.
type DumpStaging struct {
	dir     string
	entries []DumpEntry
	files   []*os.File
}

// Entries returns the dump entries to pass to [Create].
func (s *DumpStaging) Entries() []DumpEntry {
	return s.entries
}

// Size returns the total size in bytes of the staged dumps.
func (s *DumpStaging) Size() int64 {
	var size int64
	for _, file := range s.files {
		if info, err := file.Stat(); err == nil {
			size += info.Size()
		}
	}
	return size
}

// Close closes the staged dump readers and removes the staging directory.
// Close and remove failures are returned rather than swallowed, but the
// caller is expected to invoke Close from a deferred cleanup path and only
// log the error: it must not mask the result of the operation Close is
// cleaning up after.
func (s *DumpStaging) Close() error {
	var errs []error
	for _, file := range s.files {
		if err := file.Close(); err != nil {
			errs = append(errs, errors.Errorf("closing staged dump %q: %w", file.Name(), err))
		}
	}
	if err := os.RemoveAll(s.dir); err != nil {
		errs = append(errs, errors.Errorf("removing staging directory %q: %w", s.dir, err))
	}
	return errors.Capture(errors.Join(errs...))
}

// StageDumps marshals each dump into a temporary directory inside dir and
// returns the staging handle holding the resulting entries. The caller must
// create dir beforehand and must keep the handle open until the dumps have
// been archived, then Close it.
func StageDumps(ctx context.Context, dir string, dumps []NamedDump) (*DumpStaging, error) {
	stagingDir, err := os.MkdirTemp(dir, "juju-backup-dump-")
	if err != nil {
		return nil, errors.Capture(err)
	}

	staging := &DumpStaging{dir: stagingDir}
	// On any failure the readers opened so far are closed and the staging
	// directory removed; on success ownership passes to the caller.
	defer func() {
		if staging != nil {
			// The staging directory is internal to StageDumps here, so a
			// cleanup failure on this path has no caller to report to.
			_ = staging.Close()
		}
	}()

	for _, dump := range dumps {
		file, err := stageDump(ctx, stagingDir, dump.Name, dump.Export)
		if err != nil {
			return nil, err
		}
		staging.files = append(staging.files, file)
		staging.entries = append(staging.entries, DumpEntry{
			Name:   dump.Name,
			Reader: file,
		})
	}

	result := staging
	staging = nil
	return result, nil
}

// stageDump marshals one dump to a temporary file under dir and reopens it
// for reading.
func stageDump(ctx context.Context, dir, name string, export DumpExportFunc) (*os.File, error) {
	target := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return nil, errors.Capture(err)
	}
	file, err := os.Create(target)
	if err != nil {
		return nil, errors.Capture(err)
	}
	if err := export(ctx, file); err != nil {
		_ = file.Close()
		return nil, errors.Capture(err)
	}
	if err := file.Close(); err != nil {
		return nil, errors.Capture(err)
	}

	file, err = os.Open(target)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return file, nil
}

// YAMLDump returns a DumpExportFunc that streams v as YAML into the writer.
func YAMLDump(v any) DumpExportFunc {
	return func(_ context.Context, writer io.Writer) error {
		encoder := yaml.NewEncoder(writer)
		if err := encoder.Encode(v); err != nil {
			return errors.Capture(err)
		}
		return errors.Capture(encoder.Close())
	}
}

// WalkDumps reads every staged dump back from dir, passing each file's
// archive-relative name and contents to fn. It is the read side of
// [StageDumps], used when restoring a backup.
func WalkDumps(ctx context.Context, dir string, fn func(name string, reader io.Reader) error) error {
	return errors.Capture(filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name, err := filepath.Rel(dir, path)
		if err != nil {
			return errors.Capture(err)
		}
		file, err := os.Open(path)
		if err != nil {
			return errors.Capture(err)
		}
		defer file.Close()
		return fn(filepath.ToSlash(name), file)
	}))
}
