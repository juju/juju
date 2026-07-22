// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/utils/v3"
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
func NewManifestDeployer(charmPath, dataPath string, bundles BundleReader, logger Logger) Deployer {
	return &manifestDeployer{
		charmPath: charmPath,
		dataPath:  dataPath,
		bundles:   bundles,
		logger:    logger,
	}
}

type manifestDeployer struct {
	charmPath string
	dataPath  string
	bundles   BundleReader
	logger    Logger
	staged    struct {
		url      string
		bundle   Bundle
		manifest set.Strings
	}
}

func (d *manifestDeployer) Stage(info BundleInfo, abort <-chan struct{}) error {
	bdr := RetryingBundleReader{
		BundleReader: d.bundles,
		Clock:        clock.WallClock,
		Logger:       d.logger,
	}
	bundle, err := bdr.Read(info, abort)
	if err != nil {
		return err
	}
	manifest, err := bundle.ArchiveMembers()
	if err != nil {
		return err
	}
	// A charm archive controls its own member names, so validate them before
	// they are stored: an unsafe entry would poison the manifest and direct
	// later removals outside the charm directory.
	manifest, err = validateManifest(manifest)
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
	if d.staged.url == "" {
		return fmt.Errorf("charm deployment failed: no charm set")
	}

	// Detect and resolve state of charm directory.
	baseURL, baseManifest, err := d.loadManifest(CharmURLPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	upgrading := baseURL != ""
	defer func(err *error) {
		if *err != nil {
			if upgrading {
				// We now treat any failure to overwrite the charm -- or otherwise
				// manipulate the charm directory -- as a conflict, because it's
				// actually plausible for a user (or at least a charm author, who
				// is the real audience for this case) to get in there and fix it.
				d.logger.Errorf("cannot upgrade charm: %v", *err)
				*err = ErrConflict
			} else {
				// ...but if we can't install at all, we just fail out as the old
				// gitDeployer did, because I'm not willing to mess around with
				// the uniter to enable ErrConflict handling on install. We've
				// never heard of it actually happening, so this is probably not
				// a big deal.
				*err = errors.Annotate(*err, "cannot install charm")
			}
		}
	}(&err)

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
	d.logger.Debugf("deploying charm %q", d.staged.url)
	if err := d.staged.bundle.ExpandTo(d.charmPath); err != nil {
		return err
	}

	// Move the deploying file over the charm URL file, and we're done.
	return d.finishDeploy()
}

// startDeploy persists the fact that we've started deploying the staged bundle.
func (d *manifestDeployer) startDeploy() error {
	d.logger.Debugf("preparing to deploy charm %q", d.staged.url)
	if err := os.MkdirAll(d.charmPath, 0755); err != nil {
		return err
	}
	return WriteCharmURL(d.CharmPath(deployingURLPath), d.staged.url)
}

// removeDiff removes every path in oldManifest that is not present in newManifest.
func (d *manifestDeployer) removeDiff(oldManifest, newManifest set.Strings) error {
	diff := oldManifest.Difference(newManifest)
	// Manifests are validated when they are ingested, but re-validate the
	// diff here so an unsafe entry can never direct os.RemoveAll outside the
	// charm directory.
	diff, err := validateManifest(diff)
	if err != nil {
		d.logger.Warningf("skipping removal of manifest entries: %v", err)
	}
	for _, path := range diff.SortedValues() {
		fullPath := filepath.Join(d.charmPath, filepath.FromSlash(path))
		if err := os.RemoveAll(fullPath); err != nil {
			return err
		}
	}
	return nil
}

// validateManifest returns the subset of manifest entries that are safe to
// join onto the charm directory. A charm archive controls its own member
// names, so an entry like "../../x" or an absolute path would reach outside
// the charm directory, and an entry that cleans to "." would remove the charm
// directory itself. Unsafe entries are dropped from the returned set and
// reported in the error.
func validateManifest(manifest set.Strings) (set.Strings, error) {
	valid := set.NewStrings()
	var invalid []string
	for _, path := range manifest.SortedValues() {
		relPath := filepath.FromSlash(path)
		if !filepath.IsLocal(relPath) || filepath.Clean(relPath) == "." {
			invalid = append(invalid, path)
			continue
		}
		valid.Add(path)
	}
	if len(invalid) > 0 {
		return valid, errors.Errorf("charm manifest contains unsafe paths %q", invalid)
	}
	return valid, nil
}

// finishDeploy persists the fact that we've finished deploying the staged bundle.
func (d *manifestDeployer) finishDeploy() error {
	d.logger.Debugf("finishing deploy of charm %q", d.staged.url)
	oldPath := d.CharmPath(deployingURLPath)
	newPath := d.CharmPath(CharmURLPath)
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
		d.logger.Infof("detected interrupted deploy of charm %q", deployingURL)
		if deployingURL != d.staged.url {
			d.logger.Infof("removing files from charm %q", deployingURL)
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
func (d *manifestDeployer) storeManifest(url string, manifest set.Strings) error {
	if err := os.MkdirAll(d.DataPath(manifestsDataPath), 0755); err != nil {
		return err
	}
	name := charm.Quote(url)
	path := filepath.Join(d.DataPath(manifestsDataPath), name)
	return utils.WriteYaml(path, manifest.SortedValues())
}

// loadManifest loads, from dataPath, the manifest for the charm identified by the
// identity file at the supplied path within the charm directory.
func (d *manifestDeployer) loadManifest(urlFilePath string) (string, set.Strings, error) {
	url, err := ReadCharmURL(d.CharmPath(urlFilePath))
	if err != nil {
		return "", nil, err
	}
	name := charm.Quote(url)
	path := filepath.Join(d.DataPath(manifestsDataPath), name)
	manifest := []string{}
	err = utils.ReadYaml(path, &manifest)
	if os.IsNotExist(err) {
		d.logger.Warningf("manifest not found at %q: files from charm %q may be left unremoved", path, url)
		err = nil
	}
	// Manifests are validated before they are stored, but one written by an
	// older agent (or tampered with on disk) may still hold unsafe entries;
	// drop them rather than fail, so the deploy can still proceed.
	valid, invalidErr := validateManifest(set.NewStrings(manifest...))
	if invalidErr != nil {
		d.logger.Warningf("ignoring entries in stored manifest for charm %q: %v", url, invalidErr)
	}
	return url, valid, err
}

// CharmPath returns the supplied path joined to the ManifestDeployer's charm directory.
func (d *manifestDeployer) CharmPath(path string) string {
	return filepath.Join(d.charmPath, path)
}

// DataPath returns the supplied path joined to the ManifestDeployer's data directory.
func (d *manifestDeployer) DataPath(path string) string {
	return filepath.Join(d.dataPath, path)
}

type RetryingBundleReader struct {
	BundleReader

	Clock  clock.Clock
	Logger Logger
}

func (rbr RetryingBundleReader) Read(bi BundleInfo, abort <-chan struct{}) (Bundle, error) {
	var (
		bundle   Bundle
		minDelay = 200 * time.Millisecond
		maxDelay = 8 * time.Second
	)

	fetchErr := retry.Call(retry.CallArgs{
		Attempts:    10,
		Delay:       minDelay,
		BackoffFunc: retry.ExpBackoff(minDelay, maxDelay, 2.0, true),
		Clock:       rbr.Clock,
		Func: func() error {
			b, err := rbr.BundleReader.Read(bi, abort)
			if err != nil {
				return err
			}
			bundle = b
			return nil
		},
		IsFatalError: func(err error) bool {
			return err != nil && !errors.IsNotYetAvailable(err)
		},
	})

	if fetchErr != nil {
		// If the charm is still not available something went wrong.
		// Report a NotFound error instead
		if errors.Is(fetchErr, errors.NotYetAvailable) {
			rbr.Logger.Errorf("exceeded max retry attempts while waiting for blob data for %q to become available", bi.URL())
			fetchErr = fmt.Errorf("blob data for %q %w", bi.URL(), errors.NotFound)
		}
		return nil, errors.Trace(fetchErr)
	}
	return bundle, nil
}
