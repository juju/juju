// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/docker"
)

// ValidateResources runs the validation checks for resource metadata
// for each resource. Errors are consolidated and reported in a single error.
func ValidateResources(resources map[string]charmresource.Meta) error {
	var errs []error
	for _, meta := range resources {
		if err := meta.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 1 {
		return errors.Trace(errs[0])
	}
	if len(errs) > 1 {
		msgs := make([]string, len(errs))
		for i, err := range errs {
			msgs[i] = err.Error()
		}
		return errors.NewNotValid(nil, strings.Join(msgs, ", "))
	}
	return nil
}

// getDockerDetailsData determines if path is a local file path and extracts the
// details from that otherwise path is considered to be a registry path.
func getDockerDetailsData(path string, osOpen osOpenFunc) (docker.DockerImageDetails, error) {
	f, err := osOpen(path)
	if err == nil {
		defer f.Close()
		details, err := unMarshalDockerDetails(f)
		if err != nil {
			return details, errors.Trace(err)
		}
		return details, nil
	} else if err := docker.ValidateDockerRegistryPath(path); err == nil {
		return docker.DockerImageDetails{
			RegistryPath: path,
		}, nil
	}
	return docker.DockerImageDetails{}, errors.NotValidf("filepath or registry path: %s", path)

}

// ValidateResourceDetails validates the resource path in detail depending on if it's a local file or a container image,
// either checks with the FS and stats the file or validate the docker registry path and makes sure the registry URL
// resolves into a fully qualified reference.
func ValidateResourceDetails(res map[string]string, resMeta map[string]charmresource.Meta, fs modelcmd.Filesystem) error {
	for name, value := range res {
		var err error
		switch resMeta[name].Type {
		case charmresource.TypeFile:
			err = utils.CheckFile(name, value, fs)
		case charmresource.TypeContainerImage:
			var dockerDetails docker.DockerImageDetails
			dockerDetails, err = getDockerDetailsData(value, fs.Open)
			if err != nil {
				return errors.Annotatef(err, "resource %q", name)
			}
			// At the moment this is the same validation that occurs in getDockerDetailsData
			err = docker.CheckDockerDetails(name, dockerDetails)
		default:
			return fmt.Errorf("unknown resource: %s", name)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type osOpenFunc func(path string) (modelcmd.ReadSeekCloser, error)

// OpenResource returns a readable buffer for the given resource, which can be a local file or a docker image
func OpenResource(resValue string, resType charmresource.Type, osOpen osOpenFunc) (modelcmd.ReadSeekCloser, error) {
	switch resType {
	case charmresource.TypeFile:
		f, err := osOpen(resValue)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return f, nil
	case charmresource.TypeContainerImage:
		dockerDetails, err := getDockerDetailsData(resValue, osOpen)
		if err != nil {
			return nil, errors.Trace(err)
		}
		data, err := yaml.Marshal(dockerDetails)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return noopCloser{bytes.NewReader(data)}, nil
	default:
		return nil, errors.Errorf("unknown resource type %q", resType)
	}
}

type noopCloser struct {
	io.ReadSeeker
}

func (noopCloser) Close() error {
	return nil
}

func unMarshalDockerDetails(data io.Reader) (docker.DockerImageDetails, error) {
	var details docker.DockerImageDetails
	contents, err := io.ReadAll(data)
	if err != nil {
		return details, errors.Trace(err)
	}

	if errJ := json.Unmarshal(contents, &details); errJ != nil {
		if errY := yaml.Unmarshal(contents, &details); errY != nil {
			contentType := http.DetectContentType(contents)
			if strings.Contains(contentType, "text/plain") {
				// Check first character - `{` means probably JSON
				if strings.TrimSpace(string(contents))[0] == '{' {
					return details, errors.Annotate(errJ, "json parsing")
				}
				return details, errY
			}
			return details, errors.New("expected json or yaml file containing oci-image registry details")
		}
	}
	if err := docker.ValidateDockerRegistryPath(details.RegistryPath); err != nil {
		return docker.DockerImageDetails{}, err
	}
	return details, nil
}

// CheckExpectedResources compares the resources we expect to see (metadata) against what we see in the actual
// deployment arguments (the filenames and revisions), and identifies the resources that we weren't expecting.
// Note that this is different from checking if we see all the resources we expect to see, as the user can
// attach-resource post deploy.
func CheckExpectedResources(filenames map[string]string, revisions map[string]int, resMeta map[string]charmresource.Meta) error {
	var unknown []string
	for name := range filenames {
		if _, ok := resMeta[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	for name := range revisions {
		if _, ok := resMeta[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 1 {
		return errors.Errorf("unrecognized resource %q", unknown[0])
	}
	if len(unknown) > 1 {
		return errors.Errorf("unrecognized resources: %s", strings.Join(unknown, ", "))
	}
	return nil
}
