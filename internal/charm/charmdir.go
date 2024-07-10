// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
)

// defaultJujuIgnore contains jujuignore directives for excluding VCS- and
// build-related directories when archiving. The following set of directives
// will be prepended to the contents of the charm's .jujuignore file if one is
// provided.
//
// NOTE: writeArchive auto-generates its own revision and version files so they
// need to be excluded here to prevent anyone from overriding their contents by
// adding files with the same name to their charm repo.
var defaultJujuIgnore = `
.git
.svn
.hg
.bzr
.tox

/build/
/revision
/version

.jujuignore
`

// ReadOption represents an option that can be applied to a CharmDir.
type ReadOption func(*readOptions)

// WithLogger sets the logger for the CharmDir.
func WithLogger(logger logger.Logger) ReadOption {
	return func(opts *readOptions) {
		opts.logger = logger
	}
}

type readOptions struct {
	logger logger.Logger
}

func newReadOptions(options []ReadOption) *readOptions {
	opts := &readOptions{
		logger: internallogger.GetLogger("juju.charm"),
	}
	for _, option := range options {
		option(opts)
	}
	return opts
}

// CharmDir encapsulates access to data and operations
// on a charm directory.
type CharmDir struct {
	*charmBase

	Path   string
	logger logger.Logger
}

// Trick to ensure *CharmDir implements the Charm interface.
var _ Charm = (*CharmDir)(nil)

// IsCharmDir report whether the path is likely to represent
// a charm, even it may be incomplete.
func IsCharmDir(path string) bool {
	dir := &CharmDir{Path: path}
	_, err := os.Stat(dir.join("metadata.yaml"))
	return err == nil
}

// ReadCharmDir returns a CharmDir representing an expanded charm directory.
func ReadCharmDir(path string, options ...ReadOption) (*CharmDir, error) {
	opts := newReadOptions(options)

	b := &CharmDir{
		Path:      path,
		charmBase: &charmBase{},
		logger:    opts.logger,
	}
	reader, err := os.Open(b.join("metadata.yaml"))
	if err != nil {
		return nil, errors.Annotatef(err, `reading "metadata.yaml" file`)
	}
	b.meta, err = ReadMeta(reader)
	_ = reader.Close()
	if err != nil {
		return nil, errors.Annotatef(err, `parsing "metadata.yaml" file`)
	}

	// Try to read the manifest.yaml, it's required to determine if
	// this charm is v1 or not.
	reader, err = os.Open(b.join("manifest.yaml"))
	if _, ok := err.(*os.PathError); ok {
		b.manifest = nil
	} else if err != nil {
		return nil, errors.Annotatef(err, `reading "manifest.yaml" file`)
	} else {
		b.manifest, err = ReadManifest(reader)
		_ = reader.Close()
		if err != nil {
			return nil, errors.Annotatef(err, `parsing "manifest.yaml" file`)
		}
	}

	reader, err = os.Open(b.join("config.yaml"))
	if _, ok := err.(*os.PathError); ok {
		b.config = NewConfig()
	} else if err != nil {
		return nil, errors.Annotatef(err, `reading "config.yaml" file`)
	} else {
		b.config, err = ReadConfig(reader)
		_ = reader.Close()
		if err != nil {
			return nil, errors.Annotatef(err, `parsing "config.yaml" file`)
		}
	}

	if b.actions, err = getActions(
		b.meta.Name,
		func(file string) (io.ReadCloser, error) {
			return os.Open(b.join(file))
		},
		func(err error) bool {
			_, ok := err.(*os.PathError)
			return ok
		},
	); err != nil {
		return nil, err
	}

	if reader, err = os.Open(b.join("revision")); err == nil {
		_, err = fmt.Fscan(reader, &b.revision)
		_ = reader.Close()
		if err != nil {
			return nil, errors.New("invalid revision file")
		}
	}

	reader, err = os.Open(b.join("lxd-profile.yaml"))
	if _, ok := err.(*os.PathError); ok {
		b.lxdProfile = NewLXDProfile()
	} else if err != nil {
		return nil, errors.Annotatef(err, `reading "lxd-profile.yaml" file`)
	} else {
		b.lxdProfile, err = ReadLXDProfile(reader)
		_ = reader.Close()
		if err != nil {
			return nil, errors.Annotatef(err, `parsing "lxd-profile.yaml" file`)
		}
	}

	reader, err = os.Open(b.join("version"))
	if err != nil {
		if _, ok := err.(*os.PathError); !ok {
			return nil, errors.Annotatef(err, `reading "version" file`)
		}
	} else {
		b.version, err = ReadVersion(reader)
		_ = reader.Close()
		if err != nil {
			return nil, errors.Annotatef(err, `parsing "version" file`)
		}
	}

	return b, nil
}

// buildIgnoreRules parses the contents of the charm's .jujuignore file and
// compiles a set of rules that are used to decide which files should be
// archived.
func (dir *CharmDir) buildIgnoreRules() (ignoreRuleset, error) {
	// Start with a set of sane defaults to ensure backwards-compatibility
	// for charms that do not use a .jujuignore file.
	rules, err := newIgnoreRuleset(strings.NewReader(defaultJujuIgnore))
	if err != nil {
		return nil, err
	}

	pathToJujuignore := dir.join(".jujuignore")
	if _, err := os.Stat(pathToJujuignore); err == nil {
		file, err := os.Open(dir.join(".jujuignore"))
		if err != nil {
			return nil, errors.Annotatef(err, `reading ".jujuignore" file`)
		}
		defer func() { _ = file.Close() }()

		jujuignoreRules, err := newIgnoreRuleset(file)
		if err != nil {
			return nil, errors.Annotate(err, `parsing ".jujuignore" file`)
		}

		rules = append(rules, jujuignoreRules...)
	}

	return rules, nil
}

// join builds a path rooted at the charm's expanded directory
// path and the extra path components provided.
func (dir *CharmDir) join(parts ...string) string {
	parts = append([]string{dir.Path}, parts...)
	return filepath.Join(parts...)
}

// SetDiskRevision does the same as SetRevision but also changes
// the revision file in the charm directory.
func (dir *CharmDir) SetDiskRevision(revision int) error {
	dir.SetRevision(revision)
	file, err := os.OpenFile(dir.join("revision"), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	_, err = file.Write([]byte(strconv.Itoa(revision)))
	file.Close()
	return err
}

// resolveSymlinkedRoot returns the target destination of a
// charm root directory if the root directory is a symlink.
func resolveSymlinkedRoot(rootPath string) (string, error) {
	info, err := os.Lstat(rootPath)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		rootPath, err = filepath.EvalSymlinks(rootPath)
		if err != nil {
			return "", fmt.Errorf("cannot read path symlink at %q: %v", rootPath, err)
		}
	}
	return rootPath, nil
}

// ArchiveTo creates a charm file from the charm expanded in dir.
// By convention a charm archive should have a ".charm" suffix.
func (dir *CharmDir) ArchiveTo(w io.Writer) error {
	ignoreRules, err := dir.buildIgnoreRules()
	if err != nil {
		return err
	}
	// We update the version to make sure we don't lag behind
	dir.version, _, err = dir.MaybeGenerateVersionString(context.Background(), DefaultVersionReader(dir.logger))
	if err != nil {
		// We don't want to stop, even if the version cannot be generated
		dir.logger.Warningf("trying to generate version string: %v", err)
	}

	return writeArchive(w, dir.Path, dir.revision, dir.version, dir.Meta().Hooks(), ignoreRules, dir.logger)
}

func writeArchive(
	w io.Writer,
	path string,
	revision int,
	versionString string,
	hooks map[string]bool,
	ignoreRules ignoreRuleset,
	logger logger.Logger,
) error {
	zipw := zip.NewWriter(w)
	defer zipw.Close()

	// The root directory may be symlinked elsewhere so
	// resolve that before creating the zip.
	rootPath, err := resolveSymlinkedRoot(path)
	if err != nil {
		return errors.Annotatef(err, "resolving symlinked root path")
	}
	zp := zipPacker{
		Writer:      zipw,
		root:        rootPath,
		hooks:       hooks,
		ignoreRules: ignoreRules,
		logger:      logger,
	}
	if revision != -1 {
		err := zp.AddFile("revision", strconv.Itoa(revision))
		if err != nil {
			return errors.Annotatef(err, "adding 'revision' file")
		}
	}
	if versionString != "" {
		err := zp.AddFile("version", versionString)
		if err != nil {
			return errors.Annotatef(err, "adding 'version' file")
		}
	}
	if err := filepath.Walk(rootPath, zp.WalkFunc()); err != nil {
		return errors.Annotatef(err, "walking charm directory")
	}
	return nil
}

type zipPacker struct {
	*zip.Writer
	root        string
	hooks       map[string]bool
	ignoreRules ignoreRuleset
	logger      logger.Logger
}

func (zp *zipPacker) WalkFunc() filepath.WalkFunc {
	return func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return errors.Annotatef(err, "visiting %q", path)
		}

		return zp.visit(path, fi)
	}
}

func (zp *zipPacker) AddFile(filename string, value string) error {
	h := &zip.FileHeader{Name: filename}
	h.SetMode(syscall.S_IFREG | 0644)
	w, err := zp.CreateHeader(h)
	if err == nil {
		_, err = w.Write([]byte(value))
	}
	return err
}

func (zp *zipPacker) visit(path string, fi os.FileInfo) error {
	relpath, err := filepath.Rel(zp.root, path)
	if err != nil {
		return errors.Annotatef(err, "finding relative path for %q", path)
	}

	// Replace any Windows path separators with "/".
	// zip file spec 4.4.17.1 says that separators are always "/" even on Windows.
	relpath = filepath.ToSlash(relpath)

	// Check if this file or dir needs to be ignored
	if zp.ignoreRules.Match(relpath, fi.IsDir()) {
		if fi.IsDir() {
			return filepath.SkipDir
		}

		return nil
	}

	method := zip.Deflate
	if fi.IsDir() {
		relpath += "/"
		method = zip.Store
	}

	mode := fi.Mode()
	if err := checkFileType(relpath, mode); err != nil {
		return errors.Annotatef(err, "checking file type %q", relpath)
	}
	if mode&os.ModeSymlink != 0 {
		method = zip.Store
	}
	h := &zip.FileHeader{
		Name:   relpath,
		Method: method,
	}

	perm := os.FileMode(0644)
	if mode&os.ModeSymlink != 0 {
		perm = 0777
	} else if mode&0100 != 0 {
		perm = 0755
	}
	if filepath.Dir(relpath) == "hooks" {
		hookName := filepath.Base(relpath)
		if _, ok := zp.hooks[hookName]; ok && !fi.IsDir() && mode&0100 == 0 {
			zp.logger.Warningf("making %q executable in charm", path)
			perm = perm | 0100
		}
	}
	h.SetMode(mode&^0777 | perm)

	w, err := zp.CreateHeader(h)
	if err != nil || fi.IsDir() {
		return errors.Annotatef(err, "creating zip header for %q", relpath)
	}
	var data []byte
	if mode&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return errors.Annotatef(err, "reading symlink target %q", path)
		}
		if err := checkSymlinkTarget(relpath, target); err != nil {
			return errors.Annotatef(err, "checking symlink target %q", target)
		}
		data = []byte(target)
		if _, err := w.Write(data); err != nil {
			return errors.Annotatef(err, "writing symlink target %q", target)
		}
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return errors.Annotatef(err, "opening file %q", path)
	}
	defer file.Close()

	_, err = io.Copy(w, file)
	return errors.Annotatef(err, "copying file %q", path)
}

func checkSymlinkTarget(symlink, target string) error {
	if filepath.IsAbs(target) {
		return fmt.Errorf("symlink %q is absolute: %q", symlink, target)
	}
	p := filepath.Join(filepath.Dir(symlink), target)
	if p == ".." || strings.HasPrefix(p, "../") {
		return fmt.Errorf("symlink %q links out of charm: %q", symlink, target)
	}
	return nil
}

func checkFileType(path string, mode os.FileMode) error {
	e := "file has an unknown type: %q"
	switch mode & os.ModeType {
	case os.ModeDir, os.ModeSymlink, 0:
		return nil
	case os.ModeNamedPipe:
		e = "file is a named pipe: %q"
	case os.ModeSocket:
		e = "file is a socket: %q"
	case os.ModeDevice:
		e = "file is a device: %q"
	}
	return fmt.Errorf(e, path)
}

// MaybeGenerateVersionString generates charm version string.
// We want to know whether parent folders use one of these vcs, that's why we
// try to execute each one of them.
// The second return value is the detected vcs type.
func (dir *CharmDir) MaybeGenerateVersionString(ctx context.Context, reader VersionReader) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return reader.ReadVersion(ctx, dir.Path)
}

type VersionType = string

const (
	versionTypeNone VersionType = ""
	versionTypeGit  VersionType = "git"
	versionTypeHg   VersionType = "hg"
	versionTypeBzr  VersionType = "bzr"
	versionTypeFile VersionType = "versionFile"
)

// VersionReader is an interface that reads the version file.
type VersionReader interface {
	ReadVersion(ctx context.Context, path string) (string, VersionType, error)
}

var (
	// ErrVersionGenerationFailed is returned when the version string
	// generation failed.
	ErrVersionGenerationFailed = errors.ConstError("version string generation failed")
)

type versionsReader struct {
	strategies [4]VersionReader
	logger     logger.Logger
}

// DefaultVersionReader returns a VersionReader that reads the version file.
func DefaultVersionReader(logger logger.Logger) VersionReader {
	return versionsReader{
		strategies: [4]VersionReader{
			gitVersionReader{logger: logger},
			hgVersionReader{logger: logger},
			bzrVersionReader{logger: logger},
			versionFileReader{logger: logger},
		},
		logger: logger,
	}
}

func (r versionsReader) ReadVersion(ctx context.Context, path string) (string, VersionType, error) {
	absPath := path
	if !filepath.IsAbs(absPath) {
		var err error
		absPath, err = filepath.Abs(path)
		if err != nil {
			return "", "", errors.Annotatef(err, "failed resolving relative path %q", path)
		}
	}

	for _, strategy := range r.strategies {
		ver, verType, err := strategy.ReadVersion(ctx, path)
		if err != nil {
			return "", verType, errors.Annotatef(err, "failed reading version from %q", absPath)
		}
		if verType != versionTypeNone {
			return ver, verType, nil
		}
	}

	r.logger.Infof("charm is not versioned, charm path %q", absPath)
	return "", versionTypeNone, nil
}

type gitVersionReader struct {
	logger logger.Logger
}

// GitVersionReader returns just a git version reader.
func GitVersionReader(logger logger.Logger) VersionReader {
	return gitVersionReader{logger: logger}
}

func (r gitVersionReader) ReadVersion(ctx context.Context, path string) (string, VersionType, error) {
	if !r.usesGit(ctx, path) {
		return "", versionTypeNone, nil
	}
	cmd := exec.CommandContext(ctx, "git", "describe", "--dirty", "--always")
	// We need to make sure that the working directory will be the one we
	// execute the commands from.
	cmd.Dir = path
	// Version string value is written to stdout if successful.
	out, err := cmd.Output()
	if err != nil {
		// We had an error but we still know that we use a vcs, hence we can
		// stop here and handle it.
		return "", versionTypeGit, fmt.Errorf("git %w: "+
			"%w\nThis means that the charm version won't show in juju status.", ErrVersionGenerationFailed, err)
	}
	output := strings.TrimSuffix(string(out), "\n")
	return output, versionTypeGit, nil

}

// usesGit first checks for the easy case of the current charmdir has a
// git folder.
// There can be cases when the charmdir actually uses git and is just a subdir,
// hence the below check.
func (r gitVersionReader) usesGit(ctx context.Context, charmPath string) bool {
	if _, err := os.Stat(filepath.Join(charmPath, ".git")); err == nil {
		return true
	}

	args := []string{"rev-parse", "--is-inside-work-tree"}
	execCmd := exec.CommandContext(ctx, "git", args...)
	execCmd.Dir = charmPath

	if _, err := execCmd.Output(); err == nil {
		return true
	}
	return false
}

type hgVersionReader struct {
	logger logger.Logger
}

func (r hgVersionReader) ReadVersion(ctx context.Context, path string) (string, VersionType, error) {
	if !r.usesHg(path) {
		return "", versionTypeNone, nil
	}

	cmd := exec.CommandContext(ctx, "hg", "id", "-n")
	// We need to make sure that the working directory will be the one we
	// execute the commands from.
	cmd.Dir = path
	// Version string value is written to stdout if successful.
	out, err := cmd.Output()
	if err != nil {
		// We had an error but we still know that we use a vcs, hence we can
		// stop here and handle it.
		return "", versionTypeHg, fmt.Errorf("hg %w: "+
			"%w\nThis means that the charm version won't show in juju status.", ErrVersionGenerationFailed, err)
	}
	output := strings.TrimSuffix(string(out), "\n")
	return output, versionTypeHg, nil

}

// usesHg first checks for the easy case of the current charmdir has a
// hg folder.
func (r hgVersionReader) usesHg(charmPath string) bool {
	if _, err := os.Stat(filepath.Join(charmPath, ".hg")); err == nil {
		return true
	}
	return false
}

type bzrVersionReader struct {
	logger logger.Logger
}

func (r bzrVersionReader) ReadVersion(ctx context.Context, path string) (string, VersionType, error) {
	if !r.usesBzr(path) {
		return "", versionTypeNone, nil
	}

	cmd := exec.CommandContext(ctx, "bzr", "version-info")
	// We need to make sure that the working directory will be the one we
	// execute the commands from.
	cmd.Dir = path
	// Version string value is written to stdout if successful.
	out, err := cmd.Output()
	if err != nil {
		// We had an error but we still know that we use a vcs, hence we can
		// stop here and handle it.
		return "", versionTypeBzr, fmt.Errorf("bzr %w: "+
			"%w\nThis means that the charm version won't show in juju status.", ErrVersionGenerationFailed, err)
	}
	output := strings.TrimSuffix(string(out), "\n")
	return output, versionTypeBzr, nil

}

// usesBzr first checks for the easy case of the current charmdir has a
// bzr folder.
func (r bzrVersionReader) usesBzr(charmPath string) bool {
	if _, err := os.Stat(filepath.Join(charmPath, ".bzr")); err == nil {
		return true
	}
	return false
}

type versionFileReader struct {
	logger logger.Logger
}

func (r versionFileReader) ReadVersion(ctx context.Context, path string) (string, VersionType, error) {
	// If all strategies fail we fallback to check the version below
	if file, err := os.Open(filepath.Join(path, "version")); err == nil {
		r.logger.Debugf("charm is not in version control, but uses a version file")
		ver, err := ReadVersion(file)
		file.Close()
		if err != nil {
			return "", versionTypeFile, err
		}
		return ver, versionTypeFile, nil
	}

	return "", versionTypeNone, nil
}
