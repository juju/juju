// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"math/rand"
	"os/exec"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/instance"
)

// ImageURL returns a URL which can be used to download with the specified parameters from
// the state server blob store.
func ImageURL(apiInfo *Info, kind instance.ContainerType, series, arch string) (string, error) {
	serverRoot, err := serverRoot(apiInfo)
	if err != nil {
		return "", err
	}
	imageURL, err := ImageDownloadURL(kind, series, arch)
	if err != nil {
		return "", errors.Annotatef(err, "cannot determine LXC image URL: %v", err)
	}
	imageFilename := path.Base(imageURL)

	imageUrl := fmt.Sprintf(
		"%s/environment/%s/images/%v/%s/%s/%s", serverRoot, apiInfo.EnvironTag.Id(), kind, series, arch, imageFilename,
	)
	return imageUrl, nil
}

func serverRoot(apiInfo *Info) (string, error) {
	if len(apiInfo.Addrs) == 0 {
		return "", errors.New("no API host ports")
	}
	// Choose a API server at random.
	apiAddress := apiInfo.Addrs[rand.Int()%len(apiInfo.Addrs)]
	return fmt.Sprintf("https://%s", apiAddress), nil
}

// ImageDownloadURL determines the public URL which can be used to obtain an
// image blob with the specified parameters.
func ImageDownloadURL(kind instance.ContainerType, series, arch string) (string, error) {
	// TODO - we currently only need to support LXC images - kind is ignored.

	// Use the ubuntu-cloudimg-query command to get the url from which to fetch the image.
	// This will be somewhere on http://cloud-images.ubuntu.com.
	cmd := exec.Command("ubuntu-cloudimg-query", series, "released", arch, "--format", "%{url}")
	urlBytes, err := cmd.CombinedOutput()
	if err != nil {
		stderr := string(urlBytes)
		return "", errors.Annotatef(err, "cannot determine LXC image URL: %v", stderr)
	}
	imageURL := strings.Replace(string(urlBytes), ".tar.gz", "-root.tar.gz", -1)
	return imageURL, nil
}
