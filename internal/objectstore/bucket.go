// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	coreobjectstore "github.com/juju/juju/core/objectstore"
)

// ControllerBucketName returns the bucket used to store objects for a
// controller. Objects for models are namespaced under this bucket.
func ControllerBucketName(config controller.Config) (string, error) {
	name := fmt.Sprintf("juju-%s", config.ControllerUUID())
	if _, err := coreobjectstore.ParseObjectStoreBucketName(name); err != nil {
		return "", errors.Trace(err)
	}
	return name, nil
}
