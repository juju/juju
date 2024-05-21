// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"

	"github.com/juju/juju/core/logger"
)

// ResourcesHookContext is the implementation of runner.ContextResources.
type ResourcesHookContext struct {
	Client       OpenedResourceClient
	ResourcesDir string
	Logger       logger.Logger
}

// DownloadResource downloads the named resource and returns the path
// to which it was downloaded. If the resource does not exist or has
// not been uploaded yet then errors.NotFound is returned.
//
// Note that the downloaded file is checked for correctness.
func (ctx *ResourcesHookContext) DownloadResource(stdCtx context.Context, name string) (filePath string, _ error) {
	// TODO(katco): Potential race-condition: two commands running at
	// once. Solve via collision using os.Mkdir() with a uniform
	// temp dir name (e.g. "<resourcesDir>/.<res name>.download")?

	remote, err := OpenResource(stdCtx, name, ctx.Client)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer closeAndLog(remote, "remote resource", ctx.Logger)

	resPath := remote.Path
	if len(resPath) == 0 {
		return "", errors.NotValidf("empty resource path")
	}

	filePath = filepath.Join(ctx.ResourcesDir, resPath)
	isUpToDate, err := fingerprintMatches(filePath, remote.Content().Fingerprint)
	if err != nil {
		return "", errors.Trace(err)
	}
	if isUpToDate {
		// We're up-to-date already!
		return filePath, nil
	}

	if err := ctx.downloadToTarget(filePath, remote); err != nil {
		return "", errors.Trace(err)
	}

	return filePath, nil
}

func fingerprintMatches(filename string, expected charmresource.Fingerprint) (bool, error) {
	file, err := os.Open(filename)
	if os.IsNotExist(errors.Cause(err)) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	defer file.Close()

	fp, err := charmresource.GenerateFingerprint(file)
	if err != nil {
		return false, errors.Trace(err)
	}
	matches := fp.String() == expected.String()
	return matches, nil
}

// downloadToTarget saves the resource from the provided source to the target.
func (ctx *ResourcesHookContext) downloadToTarget(targetPath string, remote *OpenedResource) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return errors.Annotate(err, "could not create resource dir")
	}

	target, err := os.Create(targetPath)
	if err != nil {
		return errors.Annotate(err, "could not create new file for resource")
	}
	defer closeAndLog(target, targetPath, ctx.Logger)

	content := remote.Content()
	checker := NewContentChecker(content)
	source := checker.WrapReader(content.Data)

	_, err = io.Copy(target, source)
	if err != nil {
		return errors.Annotate(err, "could not write resource to file")
	}

	if err := checker.Verify(); err != nil {
		return errors.Trace(err)
	}
	return nil
}
