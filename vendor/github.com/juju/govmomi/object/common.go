/*
Copyright (c) 2015 VMware, Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package object

import (
	"github.com/juju/govmomi/property"
	"github.com/juju/govmomi/vim25"
	"github.com/juju/govmomi/vim25/types"
	"golang.org/x/net/context"
)

// Common contains the fields and functions common to all objects.
type Common struct {
	c *vim25.Client
	r types.ManagedObjectReference
}

func NewCommon(c *vim25.Client, r types.ManagedObjectReference) Common {
	return Common{c: c, r: r}
}

func (c Common) Reference() types.ManagedObjectReference {
	return c.r
}

func (c Common) Properties(ctx context.Context, r types.ManagedObjectReference, ps []string, dst interface{}) error {
	return property.DefaultCollector(c.c).RetrieveOne(ctx, r, ps, dst)
}
