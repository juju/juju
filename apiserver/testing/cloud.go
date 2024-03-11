// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cloud"
)

// ConstCloudGetter returns a CloudService which serves a fixed cloud.
func ConstCloudGetter(cld *cloud.Cloud) *cloudGetter {
	return &cloudGetter{cld: cld}
}

type cloudGetter struct {
	common.CloudService
	cld *cloud.Cloud
}

func (c cloudGetter) Cloud(_ context.Context, name string) (*cloud.Cloud, error) {
	if c.cld == nil {
		return nil, errors.NotFoundf("cloud %q", name)
	}
	return c.cld, nil
}
