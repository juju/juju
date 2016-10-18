// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"os"
	"path/filepath"

	"github.com/juju/utils/set"
	"github.com/juju/utils/symlink"
)

// NewDeployer returns a manifest deployer. It is a var so that it can be
// patched for uniter tests.
var NewDeployer = newDeployer

func newDeployer(charmPath, dataPath string, bundles BundleReader) (Deployer, error) {
	return NewManifestDeployer(charmPath, dataPath, bundles), nil
}

// gitManifest returns every file path in the supplied directory, *except* for:
//    * paths below .git, because we don't need to track every file: we just
//      want them all gone
//    * charmURLPath, because we don't ever want to remove that: that's how
//      the manifestDeployer keeps track of what version it's upgrading from.
// All paths are slash-separated, to match the bundle manifest format.
func gitManifest(linkPath string) (set.Strings, error) {
	dirPath, err := symlink.Read(linkPath)
	if err != nil {
		return nil, err
	}
	manifest := make(set.Strings)
	err = filepath.Walk(dirPath, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		switch relPath {
		case ".", CharmURLPath:
			return nil
		case ".git":
			err = filepath.SkipDir
		}
		manifest.Add(filepath.ToSlash(relPath))
		return err
	})
	return manifest, err
}
