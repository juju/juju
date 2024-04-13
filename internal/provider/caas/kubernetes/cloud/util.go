// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"os"

	"github.com/juju/errors"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// dataOrFile returns either the contents of data or if that is empty the
// contents of the fileName. If both are empty then no data is returned with a nil
// error
func dataOrFile(data []byte, fileName string) ([]byte, error) {
	if len(data) != 0 {
		return data, nil
	} else if fileName == "" {
		return []byte{}, nil
	}
	return os.ReadFile(fileName)
}

// PickCOntextByClusterName finds the first available context in the supplied
// kube config that is using the clusterName. If not context's are found then
// a not found error is return with an empty context name.
func PickContextByClusterName(
	config *clientcmdapi.Config,
	clusterName string,
) (string, error) {
	for contextName, context := range config.Contexts {
		if clusterName == context.Cluster {
			return contextName, nil
		}
	}
	return "", errors.NotFoundf("context for cluster name %q", clusterName)
}

// stringOrFile returns either the contents of data or if that is empty the
// contents of the fileName. If both are empty then no data is returned with a nil
// error
func stringOrFile(data string, fileName string) (string, error) {
	if len(data) != 0 {
		return data, nil
	} else if fileName == "" {
		return "", nil
	}
	d, err := os.ReadFile(fileName)
	return string(d), err
}
