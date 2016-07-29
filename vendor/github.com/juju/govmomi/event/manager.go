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

package event

import (
	"github.com/juju/govmomi/vim25"
	"github.com/juju/govmomi/vim25/methods"
	"github.com/juju/govmomi/vim25/types"
	"golang.org/x/net/context"
)

type Manager struct {
	reference types.ManagedObjectReference

	c *vim25.Client
}

func NewManager(c *vim25.Client) *Manager {
	m := Manager{
		reference: *c.ServiceContent.EventManager,

		c: c,
	}

	return &m
}

func (m Manager) CreateCollectorForEvents(ctx context.Context, filter types.EventFilterSpec) (*types.ManagedObjectReference, error) {
	req := types.CreateCollectorForEvents{
		This:   m.reference,
		Filter: filter,
	}

	res, err := methods.CreateCollectorForEvents(ctx, m.c, &req)
	if err != nil {
		return nil, err
	}

	return &res.Returnval, nil
}

func (m Manager) LogUserEvent(ctx context.Context, entity types.ManagedObjectReference, msg string) error {
	req := types.LogUserEvent{
		This:   m.reference,
		Entity: entity,
		Msg:    msg,
	}

	_, err := methods.LogUserEvent(ctx, m.c, &req)
	if err != nil {
		return err
	}

	return nil
}

func (m Manager) PostEvent(ctx context.Context, eventToPost types.BaseEvent, taskInfo types.TaskInfo) error {
	req := types.PostEvent{
		This:        m.reference,
		EventToPost: eventToPost,
		TaskInfo:    &taskInfo,
	}

	_, err := methods.PostEvent(ctx, m.c, &req)
	if err != nil {
		return err
	}

	return nil
}

func (m Manager) QueryEvents(ctx context.Context, filter types.EventFilterSpec) ([]types.BaseEvent, error) {
	req := types.QueryEvents{
		This:   m.reference,
		Filter: filter,
	}

	res, err := methods.QueryEvents(ctx, m.c, &req)
	if err != nil {
		return nil, err
	}

	return res.Returnval, nil
}

func (m Manager) RetrieveArgumentDescription(ctx context.Context, eventTypeID string) ([]types.EventArgDesc, error) {
	req := types.RetrieveArgumentDescription{
		This:        m.reference,
		EventTypeId: eventTypeID,
	}

	res, err := methods.RetrieveArgumentDescription(ctx, m.c, &req)
	if err != nil {
		return nil, err
	}

	return res.Returnval, nil
}
