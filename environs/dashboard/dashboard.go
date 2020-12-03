// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard

import (
	"archive/tar"
	"compress/bzip2"
	"encoding/json"
	"io"
	"io/ioutil"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/version"
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
		if hName != "version.json" {
			continue
		}
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
		versionStr := versInfo.Version
		vers, err = version.Parse(versionStr)
		if err != nil {
			return vers, errors.Errorf("invalid version %q in archive", versionStr)
		}
		return vers, nil
	}
	return vers, errors.New("cannot find Juju Dashboard version in archive")
}
