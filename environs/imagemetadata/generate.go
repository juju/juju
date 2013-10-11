// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"launchpad.net/juju-core/environs/simplestreams"
)

const (
	defaultIndexFileName = "index.json"
	defaultImageFileName = "imagemetadata.json"
)

// GenerateMetadata generates some basic simplestreams metadata using the specified cloud and image details.
func GenerateMetadata(series string, im *ImageMetadata, cloudSpec *simplestreams.CloudSpec, dest string) ([]string, error) {
	indexFileName := defaultIndexFileName
	imageFileName := defaultImageFileName
	now := time.Now()
	imparams := imageMetadataParams{
		Id:            im.Id,
		Arch:          im.Arch,
		Region:        cloudSpec.Region,
		URL:           cloudSpec.Endpoint,
		Path:          "streams/v1",
		ImageFileName: imageFileName,
		Updated:       now.Format(time.RFC1123Z),
		VersionKey:    now.Format("20060102"),
	}

	var err error
	imparams.Version, err = simplestreams.SeriesVersion(series)
	if err != nil {
		return nil, fmt.Errorf("invalid series %q", series)
	}

	streamsPath := filepath.Join(dest, "streams", "v1")
	if err = os.MkdirAll(streamsPath, 0755); err != nil {
		return nil, err
	}
	indexFileName = filepath.Join(streamsPath, indexFileName)
	imageFileName = filepath.Join(streamsPath, imageFileName)
	err = writeJsonFile(imparams, indexFileName, indexBoilerplate)
	if err != nil {
		return nil, err
	}
	err = writeJsonFile(imparams, imageFileName, productBoilerplate)
	if err != nil {
		return nil, err
	}
	return []string{indexFileName, imageFileName}, nil
}

type imageMetadataParams struct {
	Region        string
	URL           string
	Updated       string
	Arch          string
	Path          string
	Series        string
	Version       string
	VersionKey    string
	Id            string
	ImageFileName string
}

func writeJsonFile(imparams imageMetadataParams, filename, boilerplate string) error {
	t := template.Must(template.New("").Parse(boilerplate))
	var metadata bytes.Buffer
	if err := t.Execute(&metadata, imparams); err != nil {
		panic(fmt.Errorf("cannot generate %s metdata: %v", filename, err))
	}
	data := metadata.Bytes()
	if err := ioutil.WriteFile(filename, data, 0666); err != nil {
		return err
	}
	return nil
}

var indexBoilerplate = `
{
 "index": {
   "com.ubuntu.cloud:custom": {
     "updated": "{{.Updated}}",
     "clouds": [
       {
         "region": "{{.Region}}",
         "endpoint": "{{.URL}}"
       }
     ],
     "cloudname": "custom",
     "datatype": "image-ids",
     "format": "products:1.0",
     "products": [
       "com.ubuntu.cloud:server:{{.Version}}:{{.Arch}}"
     ],
     "path": "{{.Path}}/{{.ImageFileName}}"
   }
 },
 "updated": "{{.Updated}}",
 "format": "index:1.0"
}
`

var productBoilerplate = `
{
  "content_id": "com.ubuntu.cloud:custom",
  "format": "products:1.0",
  "updated": "{{.Updated}}",
  "datatype": "image-ids",
  "products": {
    "com.ubuntu.cloud:server:{{.Version}}:{{.Arch}}": {
      "release": "{{.Series}}",
      "version": "{{.Version}}",
      "arch": "{{.Arch}}",
      "versions": {
        "{{.VersionKey}}": {
          "items": {
            "{{.Id}}": {
              "region": "{{.Region}}",
              "id": "{{.Id}}"
            }
          },
          "pubname": "ubuntu-{{.Series}}-{{.Version}}-{{.Arch}}-server-{{.VersionKey}}",
          "label": "custom"
        }
      }
    }
  }
}
`
