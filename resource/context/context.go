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
	"github.com/juju/utils/hash"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

const HookContextFacade = resource.ComponentName + "-hook-context"

type APIClient interface {
	GetResource(resourceName string) (resource.Resource, io.ReadCloser, error)
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
	// TODO(katco): Check to see if we have latest version

	checksumWriter := charmresource.NewFingerprintHash()

	resourceInfo, resourceReader, err := c.apiClient.GetResource(name)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer resourceReader.Close()

	resourcePath := resolveResourcePath(c.unitDataDirPath, resourceInfo)
	hashingReader := hash.NewHashingReader(resourceReader, checksumWriter)
	if err := downloadAndWriteResourceToDisk(resourcePath, hashingReader); err != nil {
		return "", errors.Trace(err)
	}
	if bytes.Equal(checksumWriter.Fingerprint().Bytes(), resourceInfo.Fingerprint.Bytes()) == false {
		return "", errors.Errorf("resource fingerprint does not match expected")
	}

	return resourcePath, nil
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
	return filepath.Join(unitPath, resourceInfo.Name, resourceInfo.Path)
}

func downloadAndWriteResourceToDisk(resourcePath string, resourceReader io.Reader) error {
	// TODO(katco): Potential race-condition: two commands running at
	// once.
	if err := os.MkdirAll(filepath.Dir(resourcePath), 0755); err != nil {
		return errors.Trace(err)
	}
	resourceHandle, err := os.OpenFile(resourcePath, os.O_CREATE, 0644)
	if err != nil {
		return errors.Trace(err)
	}

	_, err = io.Copy(resourceHandle, resourceReader)
	return errors.Trace(err)
}
