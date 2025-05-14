// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charm"
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
func NewManifestDeployer(charmPath, dataPath string, bundles BundleReader, logger logger.Logger) Deployer {
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
	logger    logger.Logger
	staged    struct {
		url      string
		bundle   Bundle
		manifest set.Strings
	}
}

func (d *manifestDeployer) Stage(ctx context.Context, info BundleInfo) error {
	bdr := RetryingBundleReader{
		BundleReader: d.bundles,
		Clock:        clock.WallClock,
		Logger:       d.logger,
	}
	bundle, err := bdr.Read(ctx, info)
	if err != nil {
		return err
	}
	manifest, err := bundle.ArchiveMembers()
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
				d.logger.Errorf(context.Background(), "cannot upgrade charm: %v", *err)
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
	d.logger.Debugf(context.Background(), "deploying charm %q", d.staged.url)
	if err := d.staged.bundle.ExpandTo(d.charmPath); err != nil {
		return err
	}

	// Move the deploying file over the charm URL file, and we're done.
	return d.finishDeploy()
}

// startDeploy persists the fact that we've started deploying the staged bundle.
func (d *manifestDeployer) startDeploy() error {
	d.logger.Debugf(context.Background(), "preparing to deploy charm %q", d.staged.url)
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
	d.logger.Debugf(context.Background(), "finishing deploy of charm %q", d.staged.url)
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
		d.logger.Infof(context.Background(), "detected interrupted deploy of charm %q", deployingURL)
		if deployingURL != d.staged.url {
			d.logger.Infof(context.Background(), "removing files from charm %q", deployingURL)
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
		d.logger.Warningf(context.Background(), "manifest not found at %q: files from charm %q may be left unremoved", path, url)
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

type RetryingBundleReader struct {
	BundleReader

	Clock  clock.Clock
	Logger logger.Logger
}

func (rbr RetryingBundleReader) Read(ctx context.Context, bi BundleInfo) (Bundle, error) {
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
			b, err := rbr.BundleReader.Read(ctx, bi)
			if err != nil {
				return err
			}
			bundle = b
			return nil
		},
		IsFatalError: func(err error) bool {
			return err != nil && !errors.Is(err, errors.NotYetAvailable)
		},
		Stop: ctx.Done(),
	})

	if fetchErr != nil {
		// If the charm is still not available something went wrong.
		// Report a NotFound error instead
		if errors.Is(fetchErr, errors.NotYetAvailable) {
			rbr.Logger.Errorf(ctx, "exceeded max retry attempts while waiting for blob data for %q to become available", bi.URL())
			fetchErr = fmt.Errorf("blob data for %q %w", bi.URL(), errors.NotFound)
		}
		return nil, errors.Trace(fetchErr)
	}
	return bundle, nil
}
