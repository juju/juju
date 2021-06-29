// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/version/v2"

	"github.com/juju/juju/tools"
)

const (
	baseRegistryURL = "https://registry.hub.docker.com/v1/repositories"
)

type imageInfo struct {
	version version.Number
}

func (info imageInfo) AgentVersion() version.Number {
	return info.version
}

var logger = loggo.GetLogger("juju.docker")

// ListOperatorImages queries the standard docker registry and
// returns the version tags for images matching imagePath.
// The results are used when upgrading Juju to see what's available.
func ListOperatorImages(imagePath string) (tools.Versions, error) {
	tagsURL := fmt.Sprintf("%s/%s/tags", baseRegistryURL, imagePath)
	logger.Debugf("operater image tags URL: %v", tagsURL)
	data, err := HttpGet(tagsURL, 30*time.Second)
	if err != nil {
		return nil, errors.Trace(err)
	}

	type info struct {
		Tag string `json:"name"`
	}
	var tagInfo []info

	err = json.Unmarshal(data, &tagInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var images tools.Versions
	for _, t := range tagInfo {
		v, err := version.Parse(t.Tag)
		if err != nil {
			logger.Debugf("ignoring unexpected image tag %q", t.Tag)
			continue
		}
		images = append(images, imageInfo{v})
	}
	return images, nil
}

// Override for testing.
var HttpGet = doHttpGet

func doHttpGet(url string, timeout time.Duration) ([]byte, error) {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	defer cancelFunc()
	request = request.WithContext(ctx)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, errors.Errorf("invalid response code from registry: %v", response.StatusCode)
	}

	return ioutil.ReadAll(response.Body)
}
