// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/rpc/params"
)

// CharmInfoClient allows access to the charms API endpoint.
type CharmInfoClient struct {
	facade base.FacadeCaller
}

// NewCharmInfoClient creates a new client for accessing the charms API.
func NewCharmInfoClient(facade base.FacadeCaller) *CharmInfoClient {
	return &CharmInfoClient{facade: facade}
}

// CharmInfo returns information about the requested charm.
func (c *CharmInfoClient) CharmInfo(ctx context.Context, charmURL string) (*CharmInfo, error) {
	args := params.CharmURL{URL: charmURL}
	var info params.Charm
	if err := c.facade.FacadeCall(ctx, "CharmInfo", args, &info); err != nil {
		return nil, errors.Trace(err)
	}
	return convertCharm(&info)
}

// ApplicationCharmInfoClient allows access to the ApplicationCharmInfo endpoint.
type ApplicationCharmInfoClient struct {
	facade base.FacadeCaller
}

// NewApplicationCharmInfoClient creates a new client for accessing the
// ApplicationCharmInfo API.
func NewApplicationCharmInfoClient(facade base.FacadeCaller) *ApplicationCharmInfoClient {
	return &ApplicationCharmInfoClient{facade: facade}
}

// ApplicationCharmInfo returns information about an application's charm.
func (c *ApplicationCharmInfoClient) ApplicationCharmInfo(ctx context.Context, appName string) (*CharmInfo, error) {
	args := params.Entity{Tag: names.NewApplicationTag(appName).String()}
	var info params.Charm
	if err := c.facade.FacadeCall(ctx, "ApplicationCharmInfo", args, &info); err != nil {
		return nil, params.TranslateWellKnownError(err)
	}
	return convertCharm(&info)
}

// CharmInfo holds information about a charm.
type CharmInfo struct {
	Revision   int
	URL        string
	Config     *charm.Config
	Meta       *charm.Meta
	Actions    *charm.Actions
	Manifest   *charm.Manifest
	LXDProfile *charm.LXDProfile
	Version    string
}

func (info *CharmInfo) Charm() charm.Charm {
	return &charmImpl{info}
}

func convertCharm(info *params.Charm) (*CharmInfo, error) {
	meta, err := convertCharmMeta(info.Meta)
	if err != nil {
		return nil, errors.Trace(err)
	}
	manifest, err := convertCharmManifest(info.Manifest)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := &CharmInfo{
		Revision:   info.Revision,
		URL:        info.URL,
		Config:     params.FromCharmOptionMap(info.Config),
		Meta:       meta,
		Actions:    convertCharmActions(info.Actions),
		Manifest:   manifest,
		LXDProfile: convertCharmLXDProfile(info.LXDProfile),
		Version:    info.Version,
	}
	return result, nil
}

func convertCharmMeta(meta *params.CharmMeta) (*charm.Meta, error) {
	if meta == nil {
		return nil, nil
	}
	minVersion, err := semversion.Parse(meta.MinJujuVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}
	resources, err := convertCharmResourceMetaMap(meta.Resources)
	if err != nil {
		return nil, errors.Trace(err)
	}
	containers, err := convertCharmContainers(meta.Containers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := &charm.Meta{
		Name:           meta.Name,
		Summary:        meta.Summary,
		Description:    meta.Description,
		Subordinate:    meta.Subordinate,
		Provides:       convertCharmRelationMap(meta.Provides),
		Requires:       convertCharmRelationMap(meta.Requires),
		Peers:          convertCharmRelationMap(meta.Peers),
		ExtraBindings:  convertCharmExtraBindingMap(meta.ExtraBindings),
		Categories:     meta.Categories,
		Tags:           meta.Tags,
		Storage:        convertCharmStorageMap(meta.Storage),
		Resources:      resources,
		Terms:          meta.Terms,
		MinJujuVersion: minVersion,
		Containers:     containers,
		Assumes:        meta.AssumesExpr,
		CharmUser:      charm.RunAs(meta.CharmUser),
	}
	return result, nil
}

func convertCharmRelationMap(relations map[string]params.CharmRelation) map[string]charm.Relation {
	if len(relations) == 0 {
		return nil
	}
	result := make(map[string]charm.Relation)
	for key, value := range relations {
		result[key] = convertCharmRelation(value)
	}
	return result
}

func convertCharmRelation(relation params.CharmRelation) charm.Relation {
	return charm.Relation{
		Name:      relation.Name,
		Role:      charm.RelationRole(relation.Role),
		Interface: relation.Interface,
		Optional:  relation.Optional,
		Limit:     relation.Limit,
		Scope:     charm.RelationScope(relation.Scope),
	}
}

func convertCharmStorageMap(storage map[string]params.CharmStorage) map[string]charm.Storage {
	if len(storage) == 0 {
		return nil
	}
	result := make(map[string]charm.Storage)
	for key, value := range storage {
		result[key] = convertCharmStorage(value)
	}
	return result
}

func convertCharmStorage(storage params.CharmStorage) charm.Storage {
	return charm.Storage{
		Name:        storage.Name,
		Description: storage.Description,
		Type:        charm.StorageType(storage.Type),
		Shared:      storage.Shared,
		ReadOnly:    storage.ReadOnly,
		CountMin:    storage.CountMin,
		CountMax:    storage.CountMax,
		MinimumSize: storage.MinimumSize,
		Location:    storage.Location,
		Properties:  storage.Properties,
	}
}

func convertCharmResourceMetaMap(resources map[string]params.CharmResourceMeta) (map[string]resource.Meta, error) {
	if len(resources) == 0 {
		return nil, nil
	}
	result := make(map[string]resource.Meta)
	for key, value := range resources {
		converted, err := convertCharmResourceMeta(value)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result[key] = converted
	}
	return result, nil
}

func convertCharmResourceMeta(meta params.CharmResourceMeta) (resource.Meta, error) {
	resourceType, err := resource.ParseType(meta.Type)
	if err != nil {
		return resource.Meta{}, errors.Trace(err)
	}
	return resource.Meta{
		Name:        meta.Name,
		Type:        resourceType,
		Path:        meta.Path,
		Description: meta.Description,
	}, nil
}

func convertCharmActions(actions *params.CharmActions) *charm.Actions {
	if actions == nil {
		return nil
	}
	return &charm.Actions{
		ActionSpecs: convertCharmActionSpecMap(actions.ActionSpecs),
	}
}

func convertCharmActionSpecMap(specs map[string]params.CharmActionSpec) map[string]charm.ActionSpec {
	if len(specs) == 0 {
		return nil
	}
	result := make(map[string]charm.ActionSpec)
	for key, value := range specs {
		result[key] = convertCharmActionSpec(value)
	}
	return result
}

func convertCharmActionSpec(spec params.CharmActionSpec) charm.ActionSpec {
	return charm.ActionSpec{
		Description: spec.Description,
		Params:      spec.Params,
	}
}

func convertCharmExtraBindingMap(bindings map[string]string) map[string]charm.ExtraBinding {
	if len(bindings) == 0 {
		return nil
	}
	result := make(map[string]charm.ExtraBinding)
	for key, value := range bindings {
		result[key] = charm.ExtraBinding{Name: value}
	}
	return result
}

func convertCharmLXDProfile(lxdProfile *params.CharmLXDProfile) *charm.LXDProfile {
	if lxdProfile == nil {
		return nil
	}
	return &charm.LXDProfile{
		Description: lxdProfile.Description,
		Config:      convertCharmLXDProfileConfigMap(lxdProfile.Config),
		Devices:     convertCharmLXDProfileDevicesMap(lxdProfile.Devices),
	}
}

func convertCharmLXDProfileConfigMap(config map[string]string) map[string]string {
	result := make(map[string]string, len(config))
	for k, v := range config {
		result[k] = v
	}
	return result
}

func convertCharmLXDProfileDevicesMap(devices map[string]map[string]string) map[string]map[string]string {
	result := make(map[string]map[string]string, len(devices))
	for k, v := range devices {
		nested := make(map[string]string, len(v))
		for nk, nv := range v {
			nested[nk] = nv
		}
		result[k] = nested
	}
	return result
}

type charmImpl struct {
	info *CharmInfo
}

func (c *charmImpl) Meta() *charm.Meta {
	return c.info.Meta
}

func (c *charmImpl) Config() *charm.Config {
	return c.info.Config
}

func (c *charmImpl) Manifest() *charm.Manifest {
	return c.info.Manifest
}

func (c *charmImpl) Actions() *charm.Actions {
	return c.info.Actions
}

func (c *charmImpl) Revision() int {
	return c.info.Revision
}

func (c *charmImpl) Version() string {
	return c.info.Version
}

func convertCharmManifest(input *params.CharmManifest) (*charm.Manifest, error) {
	if input == nil {
		return nil, nil
	}
	res := []charm.Base(nil)
	for _, v := range input.Bases {
		str := fmt.Sprintf("%s@%s", v.Name, v.Channel)
		b, err := charm.ParseBase(str, v.Architectures...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		res = append(res, b)
	}
	return &charm.Manifest{Bases: res}, nil
}

func convertCharmContainers(input map[string]params.CharmContainer) (map[string]charm.Container, error) {
	containers := map[string]charm.Container{}
	for k, v := range input {
		containers[k] = charm.Container{
			Resource: v.Resource,
			Mounts:   convertCharmMounts(v.Mounts),
			Uid:      v.Uid,
			Gid:      v.Gid,
		}
	}
	if len(containers) == 0 {
		return nil, nil
	}
	return containers, nil
}

func convertCharmMounts(input []params.CharmMount) []charm.Mount {
	mounts := []charm.Mount(nil)
	for _, v := range input {
		mounts = append(mounts, charm.Mount{
			Storage:  v.Storage,
			Location: v.Location,
		})
	}
	return mounts
}
