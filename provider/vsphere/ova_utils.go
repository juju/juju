// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/juju/errors"
)

func downloadOva(basePath, url string) (string, error) {
	logger.Debugf("Downloading ova file from url: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("can't download ova file from url: %s, status: %d", url, resp.StatusCode)
	}

	ovfFilePath, err := extractOva(basePath, resp.Body)
	if err != nil {
		return "", errors.Trace(err)
	}
	file, err := os.Open(ovfFilePath)
	defer file.Close()
	if err != nil {
		return "", errors.Trace(err)
	}
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(bytes), nil
}

func extractOva(basePath string, body io.Reader) (string, error) {
	logger.Debugf("Extracting OVA to path: %s", basePath)
	tarBallReader := tar.NewReader(body)
	var ovfFileName string

	for {
		header, err := tarBallReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", errors.Trace(err)
		}
		filename := header.Name
		if filepath.Ext(filename) == ".ovf" {
			ovfFileName = filename
		}
		logger.Debugf("Writing file %s", filename)
		err = func() error {
			writer, err := os.Create(filepath.Join(basePath, filename))
			defer writer.Close()
			if err != nil {
				return errors.Trace(err)
			}
			_, err = io.Copy(writer, tarBallReader)
			if err != nil {
				return errors.Trace(err)
			}
			return nil
		}()
		if err != nil {
			return "", errors.Trace(err)
		}
	}
	if ovfFileName == "" {
		return "", errors.Errorf("no ovf file found in the archive")
	}
	logger.Debugf("OVA extracted successfully to %s", basePath)
	return filepath.Join(basePath, ovfFileName), nil
}
