/*
Copyright (c) 2014 VMware, Inc. All Rights Reserved.

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

package list

import (
	"fmt"
	"path"
	"reflect"

	"github.com/juju/govmomi/property"
	"github.com/juju/govmomi/vim25/mo"
	"github.com/juju/govmomi/vim25/types"
	"golang.org/x/net/context"
)

type Element struct {
	Path   string
	Object mo.Reference
}

func ToElement(r mo.Reference, prefix string) Element {
	var name string

	switch m := r.(type) {
	case mo.Folder:
		name = m.Name
	case mo.Datacenter:
		name = m.Name
	case mo.VirtualMachine:
		name = m.Name
	case mo.Network:
		name = m.Name
	case mo.ComputeResource:
		name = m.Name
	case mo.Datastore:
		name = m.Name
	case mo.StoragePod:
		name = m.Name
	case mo.HostSystem:
		name = m.Name
	case mo.ResourcePool:
		name = m.Name
	case mo.ClusterComputeResource:
		name = m.Name
	case mo.DistributedVirtualPortgroup:
		name = m.Name
	default:
		panic("not implemented for type " + reflect.TypeOf(r).String())
	}

	e := Element{
		Path:   path.Join(prefix, name),
		Object: r,
	}

	return e
}

type Lister struct {
	Collector *property.Collector
	Reference types.ManagedObjectReference
	Prefix    string
	All       bool
}

func traversable(ref types.ManagedObjectReference) bool {
	switch ref.Type {
	case "Folder":
	case "Datacenter":
	case "ComputeResource", "ClusterComputeResource":
		// Treat ComputeResource and ClusterComputeResource as one and the same.
		// It doesn't matter from the perspective of the lister.
	default:
		return false
	}

	return true
}

func (l Lister) retrieveProperties(ctx context.Context, req types.RetrieveProperties, dst interface{}) error {
	res, err := l.Collector.RetrieveProperties(ctx, req)
	if err != nil {
		return err
	}

	err = mo.LoadRetrievePropertiesResponse(res, dst)
	if err != nil {
		return err
	}

	return nil
}

func (l Lister) List(ctx context.Context) ([]Element, error) {
	switch l.Reference.Type {
	case "Folder":
		return l.ListFolder(ctx)
	case "Datacenter":
		return l.ListDatacenter(ctx)
	case "ComputeResource", "ClusterComputeResource":
		// Treat ComputeResource and ClusterComputeResource as one and the same.
		// It doesn't matter from the perspective of the lister.
		return l.ListComputeResource(ctx)
	case "ResourcePool":
		return l.ListResourcePool(ctx)
	default:
		return nil, fmt.Errorf("cannot traverse type " + l.Reference.Type)
	}
}

func (l Lister) ListFolder(ctx context.Context) ([]Element, error) {
	spec := types.PropertyFilterSpec{
		ObjectSet: []types.ObjectSpec{
			{
				Obj: l.Reference,
				SelectSet: []types.BaseSelectionSpec{
					&types.TraversalSpec{
						Path: "childEntity",
						Skip: types.NewBool(false),
						Type: "Folder",
					},
				},
				Skip: types.NewBool(true),
			},
		},
	}

	// Retrieve all objects that we can deal with
	childTypes := []string{
		"Folder",
		"Datacenter",
		"VirtualMachine",
		"Network",
		"ComputeResource",
		"ClusterComputeResource",
		"Datastore",
	}

	for _, t := range childTypes {
		pspec := types.PropertySpec{
			Type: t,
		}

		if l.All {
			pspec.All = types.NewBool(true)
		} else {
			pspec.PathSet = []string{"name"}

			// Additional basic properties.
			switch t {
			case "ComputeResource", "ClusterComputeResource":
				// The ComputeResource and ClusterComputeResource are dereferenced in
				// the ResourcePoolFlag. Make sure they always have their resourcePool
				// field populated.
				pspec.PathSet = append(pspec.PathSet, "resourcePool")
			}
		}

		spec.PropSet = append(spec.PropSet, pspec)
	}

	req := types.RetrieveProperties{
		SpecSet: []types.PropertyFilterSpec{spec},
	}

	var dst []interface{}

	err := l.retrieveProperties(ctx, req, &dst)
	if err != nil {
		return nil, err
	}

	es := []Element{}
	for _, v := range dst {
		es = append(es, ToElement(v.(mo.Reference), l.Prefix))
	}

	return es, nil
}

func (l Lister) ListDatacenter(ctx context.Context) ([]Element, error) {
	ospec := types.ObjectSpec{
		Obj:  l.Reference,
		Skip: types.NewBool(true),
	}

	// Include every datastore folder in the select set
	fields := []string{
		"vmFolder",
		"hostFolder",
		"datastoreFolder",
		"networkFolder",
	}

	for _, f := range fields {
		tspec := types.TraversalSpec{
			Path: f,
			Skip: types.NewBool(false),
			Type: "Datacenter",
		}

		ospec.SelectSet = append(ospec.SelectSet, &tspec)
	}

	pspec := types.PropertySpec{
		Type: "Folder",
	}

	if l.All {
		pspec.All = types.NewBool(true)
	} else {
		pspec.PathSet = []string{"name"}
	}

	req := types.RetrieveProperties{
		SpecSet: []types.PropertyFilterSpec{
			{
				ObjectSet: []types.ObjectSpec{ospec},
				PropSet:   []types.PropertySpec{pspec},
			},
		},
	}

	var dst []interface{}

	err := l.retrieveProperties(ctx, req, &dst)
	if err != nil {
		return nil, err
	}

	es := []Element{}
	for _, v := range dst {
		es = append(es, ToElement(v.(mo.Reference), l.Prefix))
	}

	return es, nil
}

func (l Lister) ListComputeResource(ctx context.Context) ([]Element, error) {
	ospec := types.ObjectSpec{
		Obj:  l.Reference,
		Skip: types.NewBool(true),
	}

	fields := []string{
		"host",
		"resourcePool",
	}

	for _, f := range fields {
		tspec := types.TraversalSpec{
			Path: f,
			Skip: types.NewBool(false),
			Type: "ComputeResource",
		}

		ospec.SelectSet = append(ospec.SelectSet, &tspec)
	}

	childTypes := []string{
		"HostSystem",
		"ResourcePool",
	}

	var pspecs []types.PropertySpec
	for _, t := range childTypes {
		pspec := types.PropertySpec{
			Type: t,
		}

		if l.All {
			pspec.All = types.NewBool(true)
		} else {
			pspec.PathSet = []string{"name"}
		}

		pspecs = append(pspecs, pspec)
	}

	req := types.RetrieveProperties{
		SpecSet: []types.PropertyFilterSpec{
			{
				ObjectSet: []types.ObjectSpec{ospec},
				PropSet:   pspecs,
			},
		},
	}

	var dst []interface{}

	err := l.retrieveProperties(ctx, req, &dst)
	if err != nil {
		return nil, err
	}

	es := []Element{}
	for _, v := range dst {
		es = append(es, ToElement(v.(mo.Reference), l.Prefix))
	}

	return es, nil
}

func (l Lister) ListResourcePool(ctx context.Context) ([]Element, error) {
	ospec := types.ObjectSpec{
		Obj:  l.Reference,
		Skip: types.NewBool(true),
	}

	fields := []string{
		"resourcePool",
	}

	for _, f := range fields {
		tspec := types.TraversalSpec{
			Path: f,
			Skip: types.NewBool(false),
			Type: "ResourcePool",
		}

		ospec.SelectSet = append(ospec.SelectSet, &tspec)
	}

	childTypes := []string{
		"ResourcePool",
	}

	var pspecs []types.PropertySpec
	for _, t := range childTypes {
		pspec := types.PropertySpec{
			Type: t,
		}

		if l.All {
			pspec.All = types.NewBool(true)
		} else {
			pspec.PathSet = []string{"name"}
		}

		pspecs = append(pspecs, pspec)
	}

	req := types.RetrieveProperties{
		SpecSet: []types.PropertyFilterSpec{
			{
				ObjectSet: []types.ObjectSpec{ospec},
				PropSet:   pspecs,
			},
		},
	}

	var dst []interface{}

	err := l.retrieveProperties(ctx, req, &dst)
	if err != nil {
		return nil, err
	}

	es := []Element{}
	for _, v := range dst {
		es = append(es, ToElement(v.(mo.Reference), l.Prefix))
	}

	return es, nil
}
