// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filepath

import (
	"runtime"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

// Renderer provides methods for the different functions in
// the stdlib path/filepath package that don't relate to a concrete
// filesystem. So Abs, EvalSymlinks, Glob, Rel, and Walk are not
// included. Also, while the functions in path/filepath relate to the
// current host, the PathRenderer methods relate to the renderer's
// target platform. So for example, a windows-oriented implementation
// will give windows-specific results even when used on linux.
type Renderer interface {
	// Base mimics path/filepath.
	Base(path string) string

	// Clean mimics path/filepath.
	Clean(path string) string

	// Dir mimics path/filepath.
	Dir(path string) string

	// Ext mimics path/filepath.
	Ext(path string) string

	// FromSlash mimics path/filepath.
	FromSlash(path string) string

	// IsAbs mimics path/filepath.
	IsAbs(path string) bool

	// Join mimics path/filepath.
	Join(path ...string) string

	// Match mimics path/filepath.
	Match(pattern, name string) (matched bool, err error)

	// NormCase normalizes the case of a pathname. On Unix and Mac OS X,
	// this returns the path unchanged; on case-insensitive filesystems,
	// it converts the path to lowercase.
	NormCase(path string) string

	// Split mimics path/filepath.
	Split(path string) (dir, file string)

	// SplitList mimics path/filepath.
	SplitList(path string) []string

	// SplitSuffix splits the pathname into a pair (root, suffix) such
	// that root + suffix == path, and ext is empty or begins with a
	// period and contains at most one period. Leading periods on the
	// basename are ignored; SplitSuffix('.cshrc') returns ('.cshrc', '').
	SplitSuffix(path string) (string, string)

	// ToSlash mimics path/filepath.
	ToSlash(path string) string

	// VolumeName mimics path/filepath.
	VolumeName(path string) string
}

// NewRenderer returns a Renderer for the given os.
func NewRenderer(os string) (Renderer, error) {
	if os == "" {
		os = runtime.GOOS
	}

	os = strings.ToLower(os)
	switch {
	case os == utils.OSWindows:
		return &WindowsRenderer{}, nil
	case utils.OSIsUnix(os):
		return &UnixRenderer{}, nil
	case os == "ubuntu":
		return &UnixRenderer{}, nil
	default:
		return nil, errors.NotFoundf("renderer for %q", os)
	}
}
