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
	indexFileName = "index.json"
	imageFileName = "imagemetadata.json"
)

func Boilerplate(series string, im *ImageMetadata, cloudSpec *CloudSpec) ([]string, error) {
	now := time.Now()
	id := imageData{
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
	id.Version, err = seriesVersion(series)
	if err != nil {
		return nil, fmt.Errorf("invalid series %q", series)
	}

	err = writeIndexJson(id)
	if err != nil {
		return nil, err
	}
	err = writeProductJson(id)
	if err != nil {
		return nil, err
	}
	return []string{indexFileName, imageFileName}, nil
}

type imageData struct {
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

func writeIndexJson(idata imageData) error {
	t := template.Must(template.New("").Parse(indexBoilerplate))
	var metadata bytes.Buffer
	if err := t.Execute(&metadata, idata); err != nil {
		panic(fmt.Errorf("cannot generate index metdata: %v", err))
	}
	data := metadata.Bytes()
	path := config.JujuHomePath(indexFileName)
	if err := ioutil.WriteFile(path, data, 0666); err != nil {
		return err
	}
	return nil
}

func writeProductJson(idata imageData) error {
	t := template.Must(template.New("").Parse(productBoilerplate))
	var metadata bytes.Buffer
	if err := t.Execute(&metadata, idata); err != nil {
		panic(fmt.Errorf("cannot generate image metdata: %v", err))
	}
	data := metadata.Bytes()
	path := config.JujuHomePath(imageFileName)
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
