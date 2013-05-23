// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"launchpad.net/juju-core/environs/config"
	"text/template"
	"time"
)

const (
	defaultIndexFileName = "index.json"
	defaultImageFileName = "imagemetadata.json"
)

func Boilerplate(name, series string, im *ImageMetadata, cloudSpec *CloudSpec) ([]string, error) {
	indexFileName := defaultIndexFileName
	imageFileName := defaultImageFileName
	if name != "" {
		indexFileName = fmt.Sprintf("%s-%s", name, indexFileName)
		imageFileName = fmt.Sprintf("%s-%s", name, imageFileName)
	}
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
	imparams.Version, err = seriesVersion(series)
	if err != nil {
		return nil, fmt.Errorf("invalid series %q", series)
	}

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
	path := config.JujuHomePath(filename)
	if err := ioutil.WriteFile(path, data, 0666); err != nil {
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
