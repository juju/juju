// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/instance"
)

// ImageURLGetter implementations provide a getter which returns a URL which
// can be used to fetch an image blob.
type ImageURLGetter interface {
	// ImageURL returns a URL which can be used to fetch an image of the
	// specified kind, series, and arch.
	ImageURL(kind instance.ContainerType, series, arch string) (string, error)

	// CACert returns the ca certificate used to validate the state server
	// certificate when using wget.
	CACert() []byte
}

type imageURLGetter struct {
	serverRoot      string
	envuuid         string
	caCert          []byte
	cloudimgBaseUrl string
}

// NewImageURLGetter returns an ImageURLGetter for the specified state
// server address and environment UUID.
func NewImageURLGetter(serverRoot, envuuid string, caCert []byte, cloudimgBaseUrl string) ImageURLGetter {
	return &imageURLGetter{
		serverRoot,
		envuuid,
		caCert,
		cloudimgBaseUrl,
	}
}

// ImageURL is specified on the NewImageURLGetter interface.
func (ug *imageURLGetter) ImageURL(kind instance.ContainerType, series, arch string) (string, error) {
	imageURL, err := ImageDownloadURL(kind, series, arch, ug.cloudimgBaseUrl)
	if err != nil {
		return "", errors.Annotatef(err, "cannot determine LXC image URL: %v", err)
	}
	imageFilename := path.Base(imageURL)

	imageUrl := fmt.Sprintf(
		"https://%s/environment/%s/images/%v/%s/%s/%s", ug.serverRoot, ug.envuuid, kind, series, arch, imageFilename,
	)
	return imageUrl, nil
}

// CACert is specified on the NewImageURLGetter interface.
func (ug *imageURLGetter) CACert() []byte {
	return ug.caCert
}

// ImageDownloadURL determines the public URL which can be used to obtain an
// image blob with the specified parameters.
func ImageDownloadURL(kind instance.ContainerType, series, arch, cloudimgBaseUrl string) (string, error) {
	// TODO - we currently only need to support LXC images - kind is ignored.
	if kind != instance.LXC {
		return "", errors.Errorf("unsupported container type: %v", kind)
	}

	// Use the ubuntu-cloudimg-query command to get the url from which to fetch the image.
	// This will be somewhere on http://cloud-images.ubuntu.com.
	cmd := exec.Command("ubuntu-cloudimg-query", series, "released", arch, "--format", "%{url}")
	if cloudimgBaseUrl != "" {
		// If the base url isn't specified, we don't need to copy the current
		// environment, because this is the default behaviour of the exec package.
		cmd.Env = append(os.Environ(), fmt.Sprintf("UBUNTU_CLOUDIMG_QUERY_BASEURL=%s", cloudimgBaseUrl))
	}
	urlBytes, err := cmd.CombinedOutput()
	if err != nil {
		stderr := string(urlBytes)
		return "", errors.Annotatef(err, "cannot determine LXC image URL: %v", stderr)
	}
	logger.Debugf("%s image for %s (%s) is %s", kind, series, arch, urlBytes)
	imageURL := strings.Replace(string(urlBytes), ".tar.gz", "-root.tar.gz", -1)
	return imageURL, nil
}
