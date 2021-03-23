// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui

import (
	"archive/tar"
	"compress/bzip2"
	"encoding/json"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/version/v2"
)

// DashboardArchiveVersion retrieves the Dashboard version
// from the given tar.bz2 archive reader.
func DashboardArchiveVersion(r io.Reader) (version.Number, error) {
	var vers version.Number
	tr := tar.NewReader(bzip2.NewReader(r))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return vers, errors.Annotate(err, "cannot read Juju Dashboard archive")
		}
		hName := hdr.Name
		if strings.HasPrefix(hName, "./") {
			hName = hName[2:]
		}
		var versionStr string
		// The new dashboard uses a version.json file.
		if hName == "version.json" {
			data, err := ioutil.ReadAll(tr)
			if err != nil {
				return vers, errors.Annotate(err, "cannot read Juju Dashboard archive version file")
			}
			type versionData struct {
				Version string `json:"version"`
			}
			var versInfo versionData
			err = json.Unmarshal(data, &versInfo)
			if err != nil {
				return vers, errors.Annotate(err, "cannot read Juju Dashboard archive version info")
			}
			versionStr = versInfo.Version
		} else {
			// Legacy archives have the version in the folder name.
			prefix := "jujugui-"
			info := hdr.FileInfo()
			if !info.IsDir() || !strings.HasPrefix(hName, prefix) {
				continue
			}
			versionStr = filepath.Dir(hName)[len(prefix):]
		}
		vers, err = version.Parse(versionStr)
		if err != nil {
			return vers, errors.Errorf("invalid version %q in archive", versionStr)
		}
		return vers, nil
	}
	return vers, errors.New("cannot find Juju Dashboard version in archive")
}
