// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"os"
	"path/filepath"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/set"
)

const (
	// deployingURLPath holds the path in the charm dir where the manifest
	// deployer writes what charm is currently being deployed.
	deployingURLPath = ".juju-deploying"

	// manifestsDataPath holds the path in the data dir where the manifest
	// deployer stores the manifests for its charms.
	manifestsDataPath = "manifests"
)

// NewManifestDeployer returns a Deployer that installs bundles from the
// supplied BundleReader into charmPath, and which reads and writes its
// persistent data into dataPath.
//
// It works by always writing the full contents of a deployed charm; and, if
// another charm was previously deployed, deleting only those files unique to
// that base charm. It thus leaves user files in place, with the exception of
// those in directories referenced only in the original charm, which will be
// deleted.
func NewManifestDeployer(charmPath, dataPath string, bundles BundleReader) Deployer {
	return &manifestDeployer{
		charmPath: charmPath,
		dataPath:  dataPath,
		bundles:   bundles,
	}
}

type manifestDeployer struct {
	charmPath string
	dataPath  string
	bundles   BundleReader
	staged    struct {
		url      *charm.URL
		bundle   Bundle
		manifest set.Strings
	}
}

func (d *manifestDeployer) Stage(info BundleInfo, abort <-chan struct{}) error {
	bundle, err := d.bundles.Read(info, abort)
	if err != nil {
		return err
	}
	manifest, err := bundle.Manifest()
	if err != nil {
		return err
	}
	url := info.URL()
	if err := d.storeManifest(url, manifest); err != nil {
		return err
	}
	d.staged.url = url
	d.staged.bundle = bundle
	d.staged.manifest = manifest
	return nil
}

func (d *manifestDeployer) Deploy() (err error) {
	if d.staged.url == nil {
		return fmt.Errorf("charm deployment failed: no charm set")
	}

	// Detect and resolve state of charm directory.
	baseURL, baseManifest, err := d.loadManifest(charmURLPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	upgrading := baseURL != nil
	defer manifestDeployError(&err, upgrading)
	if err := d.ensureBaseFiles(baseManifest); err != nil {
		return err
	}

	// Write or overwrite the deploying URL to point to the staged one.
	if err := d.startDeploy(); err != nil {
		return err
	}

	// Delete files in the base version not present in the staged charm.
	if upgrading {
		if err := d.removeDiff(baseManifest, d.staged.manifest); err != nil {
			return err
		}
	}

	// Overwrite whatever's in place with the staged charm.
	logger.Debugf("deploying charm %q", d.staged.url)
	if err := d.staged.bundle.ExpandTo(d.charmPath); err != nil {
		return err
	}

	// Move the deploying file over the charm URL file, and we're done.
	return d.finishDeploy()
}

func (d *manifestDeployer) NotifyResolved() error {
	// Maybe it is resolved, maybe not. We'll find out soon enough, but we
	// don't need to take any action now; if it's not, we'll just ErrConflict
	// out of Deploy again.
	return nil
}

func (d *manifestDeployer) NotifyRevert() error {
	// The Deploy implementation always effectively reverts when required
	// anyway, so we need take no action right now.
	return nil
}

// startDeploy persists the fact that we've started deploying the staged bundle.
func (d *manifestDeployer) startDeploy() error {
	logger.Debugf("preparing to deploy charm %q", d.staged.url)
	if err := os.MkdirAll(d.charmPath, 0755); err != nil {
		return err
	}
	return WriteCharmURL(d.CharmPath(deployingURLPath), d.staged.url)
}

// removeDiff removes every path in oldManifest that is not present in newManifest.
func (d *manifestDeployer) removeDiff(oldManifest, newManifest set.Strings) error {
	diff := oldManifest.Difference(newManifest)
	for _, path := range diff.SortedValues() {
		fullPath := filepath.Join(d.charmPath, filepath.FromSlash(path))
		if err := os.RemoveAll(fullPath); err != nil {
			return err
		}
	}
	return nil
}

// finishDeploy persists the fact that we've finished deploying the staged bundle.
func (d *manifestDeployer) finishDeploy() error {
	logger.Debugf("finishing deploy of charm %q", d.staged.url)
	oldPath := d.CharmPath(deployingURLPath)
	newPath := d.CharmPath(charmURLPath)
	return utils.ReplaceFile(oldPath, newPath)
}

// ensureBaseFiles checks for an interrupted deploy operation and, if it finds
// one, removes all entries in the manifest unique to the interrupted operation.
// This leaves files from the base charm in an indeterminate state, but ready to
// be either removed (if they are not referenced by the new charm) or overwritten
// (if they are referenced by the new charm).
//
// Note that deployingURLPath is *not* written, because the charm state remains
// indeterminate; that file will be removed when and only when a deploy completes
// successfully.
func (d *manifestDeployer) ensureBaseFiles(baseManifest set.Strings) error {
	deployingURL, deployingManifest, err := d.loadManifest(deployingURLPath)
	if err == nil {
		logger.Infof("detected interrupted deploy of charm %q", deployingURL)
		if *deployingURL != *d.staged.url {
			logger.Infof("removing files from charm %q", deployingURL)
			if err := d.removeDiff(deployingManifest, baseManifest); err != nil {
				return err
			}
		}
	}
	if os.IsNotExist(err) {
		err = nil
	}
	return err
}

// storeManifest stores, into dataPath, the supplied manifest for the supplied charm.
func (d *manifestDeployer) storeManifest(url *charm.URL, manifest set.Strings) error {
	if err := os.MkdirAll(d.DataPath(manifestsDataPath), 0755); err != nil {
		return err
	}
	name := charm.Quote(url.String())
	path := filepath.Join(d.DataPath(manifestsDataPath), name)
	return utils.WriteYaml(path, manifest.SortedValues())
}

// loadManifest loads, from dataPath, the manifest for the charm identified by the
// identity file at the supplied path within the charm directory.
func (d *manifestDeployer) loadManifest(urlFilePath string) (*charm.URL, set.Strings, error) {
	url, err := ReadCharmURL(d.CharmPath(urlFilePath))
	if err != nil {
		return nil, set.NewStrings(), err
	}
	name := charm.Quote(url.String())
	path := filepath.Join(d.DataPath(manifestsDataPath), name)
	manifest := []string{}
	err = utils.ReadYaml(path, &manifest)
	if os.IsNotExist(err) {
		logger.Warningf("manifest not found at %q: files from charm %q may be left unremoved", path, url)
		err = nil
	}
	return url, set.NewStrings(manifest...), err
}

// CharmPath returns the supplied path joined to the ManifestDeployer's charm directory.
func (d *manifestDeployer) CharmPath(path string) string {
	return filepath.Join(d.charmPath, path)
}

// DataPath returns the supplied path joined to the ManifestDeployer's data directory.
func (d *manifestDeployer) DataPath(path string) string {
	return filepath.Join(d.dataPath, path)
}

// manifestDeployError annotates or replaces the supplied error according
// to whether or not an upgrade operation is in play. It was extracted from
// Deploy to aid that method's readability.
func manifestDeployError(err *error, upgrading bool) {
	if *err != nil {
		if upgrading {
			// We now treat any failure to overwrite the charm -- or otherwise
			// manipulate the charm directory -- as a conflict, because it's
			// actually plausible for a user (or at least a charm author, who
			// is the real audience for this case) to get in there and fix it.
			logger.Errorf("cannot upgrade charm: %v", *err)
			*err = ErrConflict
		} else {
			// ...but if we can't install at all, we just fail out as the old
			// gitDeployer did, because I'm not willing to mess around with
			// the uniter to enable ErrConflict handling on install. We've
			// never heard of it actually happening, so this is probably not
			// a big deal.
			*err = fmt.Errorf("cannot install charm: %v", *err)
		}
	}
}
