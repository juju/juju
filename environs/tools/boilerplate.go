// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/juju/osenv"
)

const (
	defaultIndexFileName = "index.json"
	defaultToolsFileName = "toolsmetadata.json"
	streamsDir           = "streams/v1"
)

// Boilerplate generates some basic simplestreams metadata using the specified cloud and tools details.
func Boilerplate(tm *ToolsMetadata, cloudSpec *simplestreams.CloudSpec) ([]string, error) {
	return MakeBoilerplate(tm, cloudSpec, true)
}

// MakeBoilerplate exists so it can be called by tests. See Boilerplate above. It provides an option to retain
// the streams directories when writing the generated metadata files.
func MakeBoilerplate(tm *ToolsMetadata, cloudSpec *simplestreams.CloudSpec, flattenPath bool) ([]string, error) {
	indexFileName := defaultIndexFileName
	toolsFileName := defaultToolsFileName
	now := time.Now()
	imparams := toolsMetadataParams{
		ToolsBinarySize:   tm.Size,
		ToolsBinaryPath:   tm.Path,
		ToolsBinarySHA256: tm.SHA256,
		Version:           tm.Version,
		Arch:              tm.Arch,
		Series:            tm.Release,
		Region:            cloudSpec.Region,
		URL:               cloudSpec.Endpoint,
		Path:              streamsDir,
		ToolsFileName:     toolsFileName,
		Updated:           now.Format(time.RFC1123Z),
		VersionKey:        now.Format("20060102"),
	}

	var err error
	imparams.SeriesVersion, err = simplestreams.SeriesVersion(tm.Release)
	if err != nil {
		return nil, fmt.Errorf("invalid series %q", tm.Release)
	}

	if !flattenPath {
		streamsPath := osenv.JujuHomePath(streamsDir)
		if err := os.MkdirAll(streamsPath, 0755); err != nil {
			return nil, err
		}
		indexFileName = filepath.Join(streamsDir, indexFileName)
		toolsFileName = filepath.Join(streamsDir, toolsFileName)
	}
	err = writeJsonFile(imparams, indexFileName, indexBoilerplate)
	if err != nil {
		return nil, err
	}
	err = writeJsonFile(imparams, toolsFileName, productBoilerplate)
	if err != nil {
		return nil, err
	}
	return []string{indexFileName, toolsFileName}, nil
}

type toolsMetadataParams struct {
	ToolsBinaryPath   string
	ToolsBinarySize   int64
	ToolsBinarySHA256 string
	Region            string
	URL               string
	Updated           string
	Arch              string
	Path              string
	Series            string
	SeriesVersion     string
	Version           string
	VersionKey        string
	ToolsFileName     string
}

func writeJsonFile(imparams toolsMetadataParams, filename, boilerplate string) error {
	t := template.Must(template.New("").Parse(boilerplate))
	var metadata bytes.Buffer
	if err := t.Execute(&metadata, imparams); err != nil {
		panic(fmt.Errorf("cannot generate %s metdata: %v", filename, err))
	}
	data := metadata.Bytes()
	path := osenv.JujuHomePath(filename)
	if err := ioutil.WriteFile(path, data, 0666); err != nil {
		return err
	}
	return nil
}

var indexBoilerplate = `
{
 "index": {
   "com.ubuntu.juju:custom": {
     "updated": "{{.Updated}}",
     "cloudname": "custom",
     "datatype": "content-download",
     "format": "products:1.0",
     "products": [
       "com.ubuntu.juju:{{.SeriesVersion}}:{{.Arch}}"
     ],
     "path": "{{.Path}}/{{.ToolsFileName}}"
   }
 },
 "updated": "{{.Updated}}",
 "format": "index:1.0"
}
`

var productBoilerplate = `
{
  "content_id": "com.ubuntu.juju:custom",
  "format": "products:1.0",
  "updated": "{{.Updated}}",
  "datatype": "content-download",
  "products": {
    "com.ubuntu.juju:{{.SeriesVersion}}:{{.Arch}}": {
      "release": "{{.Series}}",
      "arch": "{{.Arch}}",
      "versions": {
        "{{.VersionKey}}": {
          "items": {
            "{{.Series}}{{.Version}}": {
              "version": "{{.Version}}",
              "size": {{.ToolsBinarySize}},
              "path": "{{.ToolsBinaryPath}}",
              "ftype": "tar.gz",
              "sha256": "{{.ToolsBinarySHA256}}"
            }
          },
          "pubname": "juju-{{.Series}}-{{.Arch}}-{{.VersionKey}}",
          "label": "custom"
        }
      }
    }
  }
}
`
