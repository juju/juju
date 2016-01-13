// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"io"
	"os"
	"path/filepath"

	"bytes"

	"github.com/juju/errors"
	"github.com/juju/juju/resource"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

const HookContextFacade = resource.ComponentName + "-hook-context"

type APIClient interface {
	GetResourceInfo(resourceName string) (resource.Resource, error)
	GetResourceDownloader(resourceName string) (io.Reader, error)
}

func NewContextAPI(apiClient APIClient, unitDataDirPath string) *Context {
	return &Context{
		apiClient:       apiClient,
		unitDataDirPath: unitDataDirPath,
	}
}

type Context struct {
	apiClient       APIClient
	unitDataDirPath string
}

func (c *Context) GetResource(name string) (string, error) {
	resourceInfo, err := getResourceInfo(name, c.apiClient)
	resourcePath := resolveResourcePath(c.unitDataDirPath, resourceInfo)

	// TODO(katco): Check to see if we have latest version

	checksumReader, checksumWriter := io.Pipe()
	defer checksumReader.Close()
	defer checksumWriter.Close()

	var fingerprint []byte
	fingerGenError := make(chan error)
	defer close(fingerGenError)
	go func() {
		fp, err := charmresource.GenerateFingerprint(checksumReader)
		if err != nil {
			fingerGenError <- err
		}
		fingerprint = fp.Bytes()
		close(fingerGenError)
	}()

	resourceReader, err := c.apiClient.GetResourceDownloader(name)
	if err != nil {
		return "", errors.Trace(err)
	}

	if err := downloadAndWriteResourceToDisk(resourcePath, resourceReader, checksumWriter); err != nil {
		return "", errors.Trace(err)
	}

	// Check that everything happened as expected.
	if err := checksumWriter.Close(); err != nil {
		// After this closes, fingerprint is correct
		return "", errors.Trace(err)
	} else if err := <-fingerGenError; err != nil {
		return "", errors.Annotate(err, "error checking resource fingerprint")
	} else if bytes.Equal(fingerprint, resourceInfo.Fingerprint.Bytes()) == false {
		return "", errors.Errorf("resource fingerprint does not match expected")
	}

	return resourcePath, nil
}

func getResourceInfo(resourceName string, apiClient APIClient) (resource.Resource, error) {
	return apiClient.GetResourceInfo(resourceName)
}

func (c *Context) Flush() error {
	return nil
}

type HookContext interface {
	GetResource(name string) (filePath string, _ error)
}

func ResourceExistsOnFilesystem(resourcePath string) (bool, error) {

	if _, err := os.Stat(resourcePath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Trace(err)
	}
	return true, nil
}

func resolveResourcePath(unitPath string, resourceInfo resource.Resource) string {
	return filepath.Join(unitPath, "resources", resourceInfo.Name, resourceInfo.Path)
}

func downloadAndWriteResourceToDisk(resourcePath string, resourceReader io.Reader, checkSumGen io.Writer) error {
	// TODO(katco): Potential race-condition: two commands running at
	// once.
	resourceReader = io.TeeReader(resourceReader, checkSumGen)
	resourceHandle, err := os.OpenFile(resourcePath, os.O_CREATE, 0644)
	if err != nil {
		return errors.Trace(err)
	}

	_, err = io.Copy(resourceHandle, resourceReader)
	return errors.Trace(err)
}
