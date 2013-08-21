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

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/simplestreams"
)

const (
	defaultIndexFileName = "index.json"
	defaultToolsFileName = "toolsmetadata.json"
	streamsDir           = "streams/v1"
)

// Boilerplate generates some basic simplestreams metadata using the specified cloud and tools details.
// If name is non-empty, it will be used as a prefix for the names of the generated index and tools files.
func Boilerplate(name, series string, tm *ToolsMetadata, cloudSpec *simplestreams.CloudSpec) ([]string, error) {
	return MakeBoilerplate(name, series, tm, cloudSpec, true)
}

// MakeBoilerplate exists so it can be called by tests. See Boilerplate above. It provides an option to retain
// the streams directories when writing the generated metadata files.
func MakeBoilerplate(name, series string, tm *ToolsMetadata, cloudSpec *simplestreams.CloudSpec, flattenPath bool) ([]string, error) {
	indexFileName := defaultIndexFileName
	toolsFileName := defaultToolsFileName
	if name != "" {
		indexFileName = fmt.Sprintf("%s-%s", name, indexFileName)
		toolsFileName = fmt.Sprintf("%s-%s", name, toolsFileName)
	}
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

	if !flattenPath {
		streamsPath := config.JujuHomePath(streamsDir)
		if err := os.MkdirAll(streamsPath, 0755); err != nil {
			return nil, err
		}
		indexFileName = filepath.Join(streamsDir, indexFileName)
		toolsFileName = filepath.Join(streamsDir, toolsFileName)
	}
	err := writeJsonFile(imparams, indexFileName, indexBoilerplate)
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
	ToolsBinarySize   float64
	ToolsBinarySHA256 string
	Region            string
	URL               string
	Updated           string
	Arch              string
	Path              string
	Series            string
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
	path := config.JujuHomePath(filename)
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
       "com.ubuntu.juju:{{.Version}}:{{.Arch}}"
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
    "com.ubuntu.juju:{{.Version}}:{{.Arch}}": {
      "release": "{{.Series}}",
      "version": "{{.Version}}",
      "arch": "{{.Arch}}",
      "versions": {
        "{{.VersionKey}}": {
          "items": {
            "{{.Series}}{{.Version}}": {
              "release": "{{.Series}}",
              "size": {{.ToolsBinarySize}},
              "path": "{{.ToolsBinaryPath}}",
              "ftype": "tar.gz",
              "sha256": "{{.ToolsBinarySHA256}}"
            }
          },
          "pubname": "juju-{{.Version}}-{{.Series}}-{{.Arch}}-{{.VersionKey}}",
          "label": "custom"
        }
      }
    }
  }
}
`
