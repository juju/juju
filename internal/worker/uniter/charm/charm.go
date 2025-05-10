// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"context"
	"errors"

	"github.com/juju/collections/set"
	"github.com/juju/utils/v4"
)

// CharmURLPath is the path within a charm directory to which Deployers
// commonly write the charm URL of the latest deployed charm.
const CharmURLPath = ".juju-charm"

// Bundle allows access to a charm's files.
type Bundle interface {

	// ArchiveMembers returns a set of slash-separated strings representing files,
	// directories, and symlinks stored in the bundle.
	ArchiveMembers() (set.Strings, error)

	// ExpandTo unpacks the entities referenced in the manifest into the
	// supplied directory. If it returns without error, every file referenced
	// in the charm must be present in the directory; implementations may vary
	// in the details of what they do with other files present.
	ExpandTo(dir string) error
}

// BundleInfo describes a Bundle.
type BundleInfo interface {

	// URL return the charm URL as a string.
	URL() string

	// ArchiveSha256 returns the hex-encoded SHA-256 digest of the bundle data.
	ArchiveSha256(context.Context) (string, error)
}

// BundleReader provides a mechanism for getting a Bundle from a BundleInfo.
type BundleReader interface {
	// Read returns the bundle identified by the supplied info. The abort chan
	// can be used to notify an implementation that it need not complete the
	// operation, and can immediately error out if it is convenient to do so.
	Read(ctx context.Context, bi BundleInfo) (Bundle, error)
}

// Deployer is responsible for installing and upgrading charms.
type Deployer interface {
	// Stage must be called to prime the Deployer to install or upgrade the
	// bundle identified by the supplied info. The abort chan can be used to
	// notify an implementation that it need not complete the operation, and
	// can immediately error out if it convenient to do so. It must always
	// be safe to restage the same bundle, or to stage a new bundle.
	Stage(ctx context.Context, info BundleInfo) error

	// Deploy will install or upgrade the most recently staged bundle.
	// Behaviour is undefined if Stage has not been called. Failures that
	// can be resolved by user intervention will be signalled by returning
	// ErrConflict.
	Deploy() error
}

// ErrConflict indicates that an upgrade failed and cannot be resolved
// without human intervention.
var ErrConflict = errors.New("charm upgrade has conflicts")

// ReadCharmURL reads a charm identity file from the supplied path.
func ReadCharmURL(path string) (string, error) {
	surl := ""
	if err := utils.ReadYaml(path, &surl); err != nil {
		return "", err
	}
	return surl, nil
}

// WriteCharmURL writes a charm identity file into the supplied path.
func WriteCharmURL(path string, url string) error {
	return utils.WriteYaml(path, url)
}
