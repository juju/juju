// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
)

var logger = loggo.GetLogger("juju.environs.imagemetadata")

// UploadImageMetadata uploads image metadata files from sourceDir to stor.
func UploadImageMetadata(stor storage.Storage, sourceDir string) error {
	if sourceDir == "" {
		return nil
	}
	metadataDir := path.Join(sourceDir, storage.BaseImagesPath, simplestreams.StreamsDir)
	info, err := os.Stat(metadataDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", sourceDir)
	}
	logger.Debugf("reading image metadata from %s", metadataDir)
	files, err := ioutil.ReadDir(metadataDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		fileName := f.Name()
		if !strings.HasSuffix(fileName, simplestreams.UnsignedSuffix) {
			continue
		}
		if err := uploadMetadataFile(stor, metadataDir, fileName, f.Size()); err != nil {
			return err
		}
	}
	return nil
}

func uploadMetadataFile(stor storage.Storage, metadataDir, fileName string, size int64) error {
	fullSourceFilename := filepath.Join(metadataDir, fileName)
	logger.Debugf("uploading metadata file %s", fileName)
	f, err := os.Open(fullSourceFilename)
	if err != nil {
		return err
	}
	defer f.Close()
	destMetadataDir := path.Join(storage.BaseImagesPath, simplestreams.StreamsDir)
	return stor.Put(path.Join(destMetadataDir, fileName), f, size)
}
