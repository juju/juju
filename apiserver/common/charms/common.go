// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/facade"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/permission"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/rpc/params"
)

// CharmService is the interface that the CharmInfoAPI requires to fetch charm
// information.
type CharmService interface {
	GetCharmID(ctx context.Context, args domaincharm.GetCharmArgs) (corecharm.ID, error)
	GetCharm(ctx context.Context, id corecharm.ID) (charm.Charm, error)
}

// CharmInfoAPI implements the charms interface and is the concrete
// implementation of the CharmInfoAPI end point.
type CharmInfoAPI struct {
	modelTag   names.ModelTag
	authorizer facade.Authorizer
	service    CharmService
}

func checkCanRead(ctx context.Context, authorizer facade.Authorizer, modelTag names.ModelTag) error {
	if authorizer.AuthController() {
		return nil
	}
	return errors.Trace(authorizer.HasPermission(ctx, permission.ReadAccess, modelTag))
}

// NewCharmInfoAPI provides the signature required for facade registration.
func NewCharmInfoAPI(service CharmService, authorizer facade.Authorizer) (*CharmInfoAPI, error) {
	return &CharmInfoAPI{
		authorizer: authorizer,
		service:    service,
	}, nil
}

// CharmInfo returns information about the requested charm.
func (a *CharmInfoAPI) CharmInfo(ctx context.Context, args params.CharmURL) (params.Charm, error) {
	if err := checkCanRead(ctx, a.authorizer, a.modelTag); err != nil {
		return params.Charm{}, errors.Trace(err)
	}

	// Parse the URL to get the charm name and revision, so that we can
	// look up the charm ID. The charm ID is used to fetch the charm
	// information.
	url, err := charm.ParseURL(args.URL)
	if err != nil {
		return params.Charm{}, errors.Trace(err)
	}

	// Get the charm ID, the charm ID is the unique UUID for the charm. All
	// operations on the charm are done using the charm ID.
	id, err := a.service.GetCharmID(ctx, domaincharm.GetCharmArgs{
		Name:     url.Name,
		Revision: ptr(url.Revision),
	})
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return params.Charm{}, errors.NotFoundf("charm %q not found", args.URL)
	} else if err != nil {
		return params.Charm{}, errors.Trace(err)
	}

	aCharm, err := a.service.GetCharm(ctx, id)
	if err != nil {
		return params.Charm{}, errors.Trace(err)
	}
	info := convertCharm(args.URL, aCharm)
	return info, nil
}

// ApplicationService is the interface that the ApplicationCharmInfoAPI
// requires to fetch charm information for an application.
type ApplicationService interface {
	GetCharmByApplicationName(context.Context, string) (charm.Charm, error)
}

// ApplicationCharmInfoAPI implements the ApplicationCharmInfo endpoint.
type ApplicationCharmInfoAPI struct {
	modelTag   names.ModelTag
	authorizer facade.Authorizer
	service    ApplicationService
}

// NewApplicationCharmInfoAPI provides the signature required for facade registration.
func NewApplicationCharmInfoAPI(modelTag names.ModelTag, service ApplicationService, authorizer facade.Authorizer) (*ApplicationCharmInfoAPI, error) {
	return &ApplicationCharmInfoAPI{
		modelTag:   modelTag,
		authorizer: authorizer,
		service:    service,
	}, nil
}

// ApplicationCharmInfo fetches charm information for an application.
func (a *ApplicationCharmInfoAPI) ApplicationCharmInfo(ctx context.Context, args params.Entity) (params.Charm, error) {
	if err := checkCanRead(ctx, a.authorizer, a.modelTag); err != nil {
		return params.Charm{}, errors.Trace(err)
	}

	appTag, err := names.ParseApplicationTag(args.Tag)
	if err != nil {
		return params.Charm{}, errors.Trace(err)
	}

	ch, err := a.service.GetCharmByApplicationName(ctx, appTag.Id())
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return params.Charm{}, errors.NotFoundf("application %q not found", appTag.Id())
	} else if errors.Is(err, applicationerrors.CharmNotFound) {
		return params.Charm{}, errors.NotFoundf("charm for application %q not found", appTag.Id())
	} else if err != nil {
		return params.Charm{}, errors.Trace(err)
	}

	return convertCharm("", ch), nil
}

func convertCharm(url string, ch charm.Charm) params.Charm {
	result := params.Charm{
		Revision: ch.Revision(),
		URL:      url,
		Config:   params.ToCharmOptionMap(ch.Config()),
		Meta:     convertCharmMeta(ch.Meta()),
		Actions:  convertCharmActions(ch.Actions()),
		Manifest: convertCharmManifest(ch.Manifest()),
	}

	profiler, ok := ch.(charm.LXDProfiler)
	if !ok {
		return result
	}

	profile := profiler.LXDProfile()
	if profile != nil && !profile.Empty() {
		result.LXDProfile = convertCharmLXDProfile(profile)
	}

	return result
}

func convertCharmMeta(meta *charm.Meta) *params.CharmMeta {
	if meta == nil {
		return nil
	}
	return &params.CharmMeta{
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
		Devices:        convertCharmDevices(meta.Devices),
		PayloadClasses: convertCharmPayloadClassMap(meta.PayloadClasses),
		Resources:      convertCharmResourceMetaMap(meta.Resources),
		Terms:          meta.Terms,
		MinJujuVersion: meta.MinJujuVersion.String(),
		Containers:     convertCharmContainers(meta.Containers),
		AssumesExpr:    meta.Assumes,
		CharmUser:      string(meta.CharmUser),
	}
}

func convertCharmManifest(manifest *charm.Manifest) *params.CharmManifest {
	if manifest == nil {
		return nil
	}
	return &params.CharmManifest{
		Bases: convertCharmBases(manifest.Bases),
	}
}

func convertCharmRelationMap(relations map[string]charm.Relation) map[string]params.CharmRelation {
	if len(relations) == 0 {
		return nil
	}
	result := make(map[string]params.CharmRelation)
	for key, value := range relations {
		result[key] = convertCharmRelation(value)
	}
	return result
}

func convertCharmRelation(relation charm.Relation) params.CharmRelation {
	return params.CharmRelation{
		Name:      relation.Name,
		Role:      string(relation.Role),
		Interface: relation.Interface,
		Optional:  relation.Optional,
		Limit:     relation.Limit,
		Scope:     string(relation.Scope),
	}
}

func convertCharmStorageMap(storage map[string]charm.Storage) map[string]params.CharmStorage {
	if len(storage) == 0 {
		return nil
	}
	result := make(map[string]params.CharmStorage)
	for key, value := range storage {
		result[key] = convertCharmStorage(value)
	}
	return result
}

func convertCharmStorage(storage charm.Storage) params.CharmStorage {
	return params.CharmStorage{
		Name:        storage.Name,
		Description: storage.Description,
		Type:        string(storage.Type),
		Shared:      storage.Shared,
		ReadOnly:    storage.ReadOnly,
		CountMin:    storage.CountMin,
		CountMax:    storage.CountMax,
		MinimumSize: storage.MinimumSize,
		Location:    storage.Location,
		Properties:  storage.Properties,
	}
}

func convertCharmPayloadClassMap(payload map[string]charm.PayloadClass) map[string]params.CharmPayloadClass {
	if len(payload) == 0 {
		return nil
	}
	result := make(map[string]params.CharmPayloadClass)
	for key, value := range payload {
		result[key] = convertCharmPayloadClass(value)
	}
	return result
}

func convertCharmPayloadClass(payload charm.PayloadClass) params.CharmPayloadClass {
	return params.CharmPayloadClass{
		Name: payload.Name,
		Type: payload.Type,
	}
}

func convertCharmResourceMetaMap(resources map[string]resource.Meta) map[string]params.CharmResourceMeta {
	if len(resources) == 0 {
		return nil
	}
	result := make(map[string]params.CharmResourceMeta)
	for key, value := range resources {
		result[key] = convertCharmResourceMeta(value)
	}
	return result
}

func convertCharmResourceMeta(meta resource.Meta) params.CharmResourceMeta {
	return params.CharmResourceMeta{
		Name:        meta.Name,
		Type:        meta.Type.String(),
		Path:        meta.Path,
		Description: meta.Description,
	}
}

func convertCharmActions(actions *charm.Actions) *params.CharmActions {
	if actions == nil {
		return nil
	}
	result := &params.CharmActions{
		ActionSpecs: convertCharmActionSpecMap(actions.ActionSpecs),
	}

	return result
}

func convertCharmActionSpecMap(specs map[string]charm.ActionSpec) map[string]params.CharmActionSpec {
	if len(specs) == 0 {
		return nil
	}
	result := make(map[string]params.CharmActionSpec)
	for key, value := range specs {
		result[key] = convertCharmActionSpec(value)
	}
	return result
}

func convertCharmActionSpec(spec charm.ActionSpec) params.CharmActionSpec {
	return params.CharmActionSpec{
		Description: spec.Description,
		Params:      spec.Params,
	}
}

func convertCharmExtraBindingMap(bindings map[string]charm.ExtraBinding) map[string]string {
	if len(bindings) == 0 {
		return nil
	}
	result := make(map[string]string)
	for key, value := range bindings {
		result[key] = value.Name
	}
	return result
}

func convertCharmLXDProfile(profile *charm.LXDProfile) *params.CharmLXDProfile {
	return &params.CharmLXDProfile{
		Description: profile.Description,
		Config:      convertCharmLXDProfileConfig(profile.Config),
		Devices:     convertCharmLXDProfileDevices(profile.Devices),
	}
}

func convertCharmLXDProfileConfig(config map[string]string) map[string]string {
	result := map[string]string{}
	for k, v := range config {
		result[k] = v
	}
	return result
}

func convertCharmLXDProfileDevices(devices map[string]map[string]string) map[string]map[string]string {
	result := map[string]map[string]string{}
	for k, v := range devices {
		nested := map[string]string{}
		for nk, nv := range v {
			nested[nk] = nv
		}
		result[k] = nested
	}
	return result
}

func convertCharmDevices(devices map[string]charm.Device) map[string]params.CharmDevice {
	if devices == nil {
		return nil
	}
	results := make(map[string]params.CharmDevice)
	for k, v := range devices {
		results[k] = params.CharmDevice{
			Name:        v.Name,
			Description: v.Description,
			Type:        string(v.Type),
			CountMin:    v.CountMin,
			CountMax:    v.CountMax,
		}
	}
	return results
}

func convertCharmBases(input []charm.Base) []params.CharmBase {
	var bases []params.CharmBase
	for _, v := range input {
		bases = append(bases, params.CharmBase{
			Name:          v.Name,
			Channel:       v.Channel.String(),
			Architectures: v.Architectures,
		})
	}
	return bases
}

func convertCharmContainers(input map[string]charm.Container) map[string]params.CharmContainer {
	containers := map[string]params.CharmContainer{}
	for k, v := range input {
		containers[k] = params.CharmContainer{
			Resource: v.Resource,
			Mounts:   convertCharmMounts(v.Mounts),
			Uid:      v.Uid,
			Gid:      v.Gid,
		}
	}
	if len(containers) == 0 {
		return nil
	}
	return containers
}

func convertCharmMounts(input []charm.Mount) []params.CharmMount {
	var mounts []params.CharmMount
	for _, v := range input {
		mounts = append(mounts, params.CharmMount{
			Storage:  v.Storage,
			Location: v.Location,
		})
	}
	return mounts
}

func ptr[T any](t T) *T {
	return &t
}
