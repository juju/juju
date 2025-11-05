// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"

	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/storage"
	jujustorage "github.com/juju/juju/internal/storage"
)

func validateStorageAttributes(attributes map[string]interface{}) error {
	if _, err := storage.ParseStorageConfig(attributes); err != nil {
		return errors.Capture(err)
	}
	if _, err := storage.ParseStorageMode(attributes); err != nil {
		return errors.Capture(err)
	}
	return nil
}

func validateStorageMedium(attributes map[string]any) error {
	if attributes == nil {
		return nil
	}

	if mediumValue, ok := attributes[constants.StorageMedium]; ok {
		medium := core.StorageMedium(fmt.Sprintf("%v", mediumValue))
		if medium != core.StorageMediumMemory && medium != core.StorageMediumHugePages {
			return errors.Errorf(
				"storage medium %q not valid", mediumValue,
			).Add(coreerrors.NotValid)
		}
	}

	return nil
}

// noopFSSource is a [jujustorage.FilesystemSource] that performs no actions for
// the provider.
type noopFSSource struct{}

// rootfsStorageProvider is a [jujustorage.Provider] that provides rootfs
// like filesystems in a Kubernetes model.
type rootfsStorageProvider struct{}

// storageProvider is a [jujustorage.Provider] that provides storage in a
// Kubernetes model.
type storageProvider struct {
	client *kubernetesClient
}

// tmpfsStorageProvider is a [jujustorage.Provider] that provides tmpfs like
// filesystems in a Kubernetes model.
type tmpfsStorageProvider struct{}

var (
	_ jujustorage.Provider = (*rootfsStorageProvider)(nil)
	_ jujustorage.Provider = (*storageProvider)(nil)
	_ jujustorage.Provider = (*tmpfsStorageProvider)(nil)
)

// RecommendedPoolForKind returns the recommended storage pool to use for
// supplied storage kind. At the moment the only supported recommended kind is
// filesystem.
//
// This func implements the [jujustorage.ProviderRegistry] interface.
func (k *kubernetesClient) RecommendedPoolForKind(
	kind jujustorage.StorageKind,
) *jujustorage.Config {
	// NOTE (tlm): The Juju logic around if a storage provider through either
	// its filesystem or volume source are capable of supporting a given storage
	// kind is not owned by a provider today. This determinantion is made by
	// storage provisoning where the assumption is that a volume source can be a
	// volume or filesystem source. This divorces the provider from being
	// involved in the decision making even though it provides the storage.
	//
	// For now we assume that the Kubernetes provider is always capable of
	// creating filesystems and block devices are not supported on Kubernetes.
	if kind != jujustorage.StorageKindFilesystem {
		return nil
	}

	s := storageProvider{k}
	defaultPools := s.DefaultPools()
	// We take the first default pool offered by the storage provider.
	if len(defaultPools) != 0 {
		return defaultPools[0]
	}
	return nil
}

// StorageProviderTypes is defined on the jujustorage.ProviderRegistry interface.
func (*kubernetesClient) StorageProviderTypes() ([]jujustorage.ProviderType, error) {
	return []jujustorage.ProviderType{
		constants.StorageProviderType,
		constants.StorageProviderTypeRootfs,
		constants.StorageProviderTypeTmpfs,
	}, nil
}

// StorageProvider is defined on the jujustorage.ProviderRegistry interface.
func (k *kubernetesClient) StorageProvider(t jujustorage.ProviderType) (jujustorage.Provider, error) {
	switch t {
	case constants.StorageProviderType:
		return &storageProvider{k}, nil
	case constants.StorageProviderTypeRootfs:
		return &rootfsStorageProvider{}, nil
	case constants.StorageProviderTypeTmpfs:
		return &tmpfsStorageProvider{}, nil
	default:
		return nil, errors.Errorf(
			"storage provider for type %q does not exist",
			t,
		).Add(coreerrors.NotFound)
	}
}

// AttachFilesystems is a noop operation for attaching filesystems in this
// source.
//
// Implements the [jujustorage.FilesystemSource] interface.
func (*noopFSSource) AttachFilesystems(
	_ context.Context, params []jujustorage.FilesystemAttachmentParams,
) ([]jujustorage.AttachFilesystemsResult, error) {
	// This is a no-op, but we must reflect the input back to the storage
	// provisioner so that it can set the provisioned state.
	results := make([]jujustorage.AttachFilesystemsResult, 0, len(params))
	for _, v := range params {
		result := jujustorage.AttachFilesystemsResult{
			FilesystemAttachment: &jujustorage.FilesystemAttachment{
				Filesystem: v.Filesystem,
				Machine:    v.Machine,
				FilesystemAttachmentInfo: jujustorage.FilesystemAttachmentInfo{
					Path:     v.Path,
					ReadOnly: v.ReadOnly,
				},
			},
		}
		results = append(results, result)
	}
	return results, nil
}

// CreateFilesystems is a noop operation for creating filesystems in this source.
//
// Implements the [jujustorage.FilesystemSource] interface.
func (*noopFSSource) CreateFilesystems(
	_ context.Context, params []jujustorage.FilesystemParams,
) ([]jujustorage.CreateFilesystemsResult, error) {
	return make([]jujustorage.CreateFilesystemsResult, len(params)), nil
}

// DefaultPools returns the default storage pools for [rootfsStorageProvider].
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) DefaultPools() []*jujustorage.Config {
	pool, _ := jujustorage.NewConfig(
		constants.StorageProviderTypeRootfs.String(),
		constants.StorageProviderTypeRootfs,
		jujustorage.Attrs{},
	)
	return []*jujustorage.Config{pool}
}

// DefaultPools returns the default storage pools for [storageProvider].
//
// Implements the [jujustorage.Provider] interface.
func (*storageProvider) DefaultPools() []*jujustorage.Config {
	pool, _ := jujustorage.NewConfig(
		constants.StorageProviderType.String(),
		constants.StorageProviderType,
		jujustorage.Attrs{},
	)
	return []*jujustorage.Config{pool}
}

// DefaultPools returns the default storage pools for [tmpfsStorageProvider].
//
// Implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) DefaultPools() []*jujustorage.Config {
	pool, _ := jujustorage.NewConfig(
		constants.StorageProviderTypeTmpfs.String(),
		constants.StorageProviderTypeTmpfs,
		jujustorage.Attrs{},
	)
	return []*jujustorage.Config{pool}
}

// DestroyFilesystems is a noop operation for destroying filesystems in this
// source.
//
// Implements the [jujustorage.FilesystemSource] interface.
func (*noopFSSource) DestroyFilesystems(
	_ context.Context, _ []string,
) ([]error, error) {
	return nil, nil
}

// DetatchFilesystems is a noop operation for detaching filesystems in this
// source.
//
// Implements the [jujustorage.FilesystemSource] interface.
func (*noopFSSource) DetachFilesystems(
	_ context.Context, params []jujustorage.FilesystemAttachmentParams,
) ([]error, error) {
	return make([]error, len(params)), nil
}

// Dynamic informs the caller if this provider supports creating storage after
// a machine is provisioned. This question is not applicable to Kubernetes so
// we return true.
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) Dynamic() bool {
	return true
}

// Dynamic informs the caller if this provider supports creating storage after
// a machine is provisioned. This question is not applicable to Kubernetes so
// we return true.
//
// Implements the [jujustorage.Provider] interface.
func (*storageProvider) Dynamic() bool {
	return true
}

// Dynamic informs the caller if this provider supports creating storage after
// a machine is provisioned. This question is not applicable to Kubernetes so
// we return true.
//
// Implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) Dynamic() bool {
	return true
}

// FilesystemSource returns the filesystem source for this provider.
//
// The returned filesystem source does not provision any filesystems in the
// model and results in noop operations. Actual provisioning is done by
// kubernetes as part of creating the application.
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) FilesystemSource(
	_ *jujustorage.Config,
) (jujustorage.FilesystemSource, error) {
	return &noopFSSource{}, nil
}

// FilesystemSource is defined on the jujustorage.Provider interface.
func (g *storageProvider) FilesystemSource(providerConfig *jujustorage.Config) (jujustorage.FilesystemSource, error) {
	return &filesystemSource{
		client: g.client,
	}, nil
}

// FilesystemSource returns the filesystem source for this provider.
//
// The returned filesystem source does not provision any filesystems in the
// model and results in noop operations. Actual provisioning is done by
// kubernetes as part of creating the application.
//
// Implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) FilesystemSource(
	_ *jujustorage.Config,
) (jujustorage.FilesystemSource, error) {
	return &noopFSSource{}, nil
}

// Reports if this provider is capable of releasing dynamically provisioned
// storage.
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) Releasable() bool {
	return false
}

// Reports if this provider is capable of releasing dynamically provisioned
// storage.
//
// Implements the [jujustorage.Provider] interface.
func (*storageProvider) Releasable() bool {
	return true
}

// Reports if this provider is capable of releasing dynamically provisioned
// storage.
//
// Implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) Releasable() bool {
	return false
}

// ReleaseFilesystems is a noop operation for releasing filesystems in this
// source.
//
// Implements the [jujustorage.FilesystemSource] interface.
func (*noopFSSource) ReleaseFilesystems(
	_ context.Context, _ []string,
) ([]error, error) {
	return nil, nil
}

// Scope returns the provisioning scope required for [rootfsStorageProvider].
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) Scope() jujustorage.Scope {
	return jujustorage.ScopeEnviron
}

// Scope returns the provisioning scope required for [storageProvider].
//
// Implements the [jujustorage.Provider] interface.
func (*storageProvider) Scope() jujustorage.Scope {
	return jujustorage.ScopeEnviron
}

// Scope returns the provisioning scope required for [tmpfsStorageProvider].
//
// Implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) Scope() jujustorage.Scope {
	return jujustorage.ScopeEnviron
}

// Supports tells the caller if this provider supports the given storage kind.
// Currently only filesystems are supported.
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) Supports(k jujustorage.StorageKind) bool {
	return k == jujustorage.StorageKindFilesystem
}

// Support tells the caller if this provider supports the given storage kind.
// Currently only block storage is supported.
//
// Implements the [jujustorage.Provider] interface.
func (g *storageProvider) Supports(k jujustorage.StorageKind) bool {
	return k == jujustorage.StorageKindFilesystem
}

// Support tells the caller if this provider supports the given storage kind.
// Currently only filesystems are supported.
//
// Implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) Supports(k jujustorage.StorageKind) bool {
	return k == jujustorage.StorageKindFilesystem
}

// ValidateConfig validates the supplied configuration for the rootfs storage
// provider. This func implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) ValidateConfig(cfg *jujustorage.Config) error {
	return errors.Capture(validateStorageMedium(cfg.Attrs()))
}

// ValidateConfig is defined on the jujustorage.Provider interface.
func (g *storageProvider) ValidateConfig(cfg *jujustorage.Config) error {
	return errors.Capture(validateStorageAttributes(cfg.Attrs()))
}

// ValidateConfig validates the supplied configuration for the tmpfs storage
// provider. This func implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) ValidateConfig(cfg *jujustorage.Config) error {
	return errors.Capture(validateStorageMedium(cfg.Attrs()))
}

func (*rootfsStorageProvider) ValidateForK8s(attributes map[string]any) error {
	return errors.Capture(validateStorageMedium(attributes))
}

// ValidateStorageProvider returns an error if the storage type and config is not valid
// for a Kubernetes deployment.
func (g *storageProvider) ValidateForK8s(attributes map[string]any) error {
	if attributes == nil {
		return nil
	}

	if err := validateStorageMedium(attributes); err != nil {
		return errors.Capture(err)
	}

	if err := validateStorageAttributes(attributes); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (*tmpfsStorageProvider) ValidateForK8s(attributes map[string]any) error {
	return errors.Capture(validateStorageMedium(attributes))
}

// ValidateFilesystemParams validates the parameters for a fielsystem. No
// validation is performed by this source and results in a noop operation.
func (*noopFSSource) ValidateFilesystemParams(_ jujustorage.FilesystemParams) error {
	return nil
}

// VolumeSource provides the [jujustorage.VolumeSource] implementation for tmpfs
// storage.
//
// This always results in an error as volumes are not support for tmpfs storage.
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) VolumeSource(*jujustorage.Config) (
	jujustorage.VolumeSource, error,
) {
	return nil, errors.New("volumes are not supported by rootfs storage").Add(
		coreerrors.NotSupported,
	)
}

// VolumeSource is defined on the jujustorage.Provider interface.
func (g *storageProvider) VolumeSource(cfg *jujustorage.Config) (jujustorage.VolumeSource, error) {
	return nil, errors.New("volumes are not supported").Add(
		coreerrors.NotSupported,
	)
}

// VolumeSource provides the [jujustorage.VolumeSource] implementation for tmpfs
// storage.
//
// This always results in an error as volumes are not support for tmpfs storage.
//
// Implements the [jujustorage.Provider] interface.
func (p *tmpfsStorageProvider) VolumeSource(*jujustorage.Config) (
	jujustorage.VolumeSource, error,
) {
	return nil, errors.New("volumes are not supported by tmpfs").Add(
		coreerrors.NotSupported,
	)
}

type filesystemSource struct {
	client *kubernetesClient
}

var _ jujustorage.FilesystemSource = (*filesystemSource)(nil)

// ValidateFilesystemParams is specified on the jujustorage.FilesystemSource interface.
func (v *filesystemSource) ValidateFilesystemParams(
	params jujustorage.FilesystemParams,
) error {
	if params.ProviderId == nil {
		return errors.Errorf(
			"kubernetes filesystem %q missing provider id", params.Tag.Id(),
		).Add(jujustorage.FilesystemCreateParamsIncomplete)
	}
	return nil
}

// CreateFilesystems is specified on the jujustorage.FilesystemSource interface.
func (v *filesystemSource) CreateFilesystems(
	ctx context.Context,
	params []jujustorage.FilesystemParams,
) ([]jujustorage.CreateFilesystemsResult, error) {
	results := make([]jujustorage.CreateFilesystemsResult, 0, len(params))
	for _, param := range params {
		var result jujustorage.CreateFilesystemsResult
		if param.ProviderId == nil {
			result.Error = errors.Errorf(
				"creating kubernetes filesystem %q with missing provider id",
				param.Tag.Id(),
			).Add(coreerrors.NotValid)
			results = append(results, result)
			continue
		}
		// Kubernetes filesystems are PersistentVolumes that are provisioned
		// by Kubernetes, not by the storage provider. Instead we check that
		// it exists and source any filesystem required information.
		fsInfo, err := v.getPersistentVolume(ctx, *param.ProviderId)
		if err != nil {
			result.Error = errors.Errorf(
				"finalising kubernetes filesystem %q with PersistentVolume %q: %w",
				param.Tag.Id(), *param.ProviderId, err,
			)
		} else {
			result.Filesystem = &jujustorage.Filesystem{
				Tag:            param.Tag,
				FilesystemInfo: fsInfo,
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func (v *filesystemSource) getPersistentVolume(
	ctx context.Context, pvName string,
) (jujustorage.FilesystemInfo, error) {
	pvAPI := v.client.client().CoreV1().PersistentVolumes()
	pv, err := pvAPI.Get(ctx, pvName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return jujustorage.FilesystemInfo{}, errors.New(
			"kubernetes PersistentVolume not found",
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return jujustorage.FilesystemInfo{}, errors.Errorf(
			"getting kubernetes PersistentVolume: %w", err,
		)
	}
	size := uint64(0)
	if pv.Spec.Capacity != nil {
		storageCapacity := pv.Spec.Capacity.Storage()
		if storageCapacity != nil {
			// TODO(storage): does this need scaling to MiB?
			size = uint64(storageCapacity.Value())
		}
	}
	fs := jujustorage.FilesystemInfo{
		ProviderId: pvName,
		Size:       size,
	}
	return fs, nil
}

// DestroyFilesystems is specified on the jujustorage.FilesystemSource interface.
func (v *filesystemSource) DestroyFilesystems(ctx context.Context, pvNames []string) ([]error, error) {
	logger.Infof(ctx, "destroying kubernetes PersistentVolume(s): %v", pvNames)
	errs := make([]error, 0, len(pvNames))
	for _, pvName := range pvNames {
		err := v.deletePersistentVolume(ctx, pvName)
		if err != nil {
			err = errors.Errorf(
				"destroying kubernetes PersistentVolume %q: %w", pvName, err,
			)
		}
		errs = append(errs, err)
	}
	return errs, nil
}

func (v *filesystemSource) deletePersistentVolume(
	ctx context.Context, pvName string,
) error {
	pvAPI := v.client.client().CoreV1().PersistentVolumes()
	err := pvAPI.Delete(ctx, pvName, v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Errorf("deleting PersistentVolume: %w", err)
	}
	return nil
}

// ReleaseFilesystems is specified on the jujustorage.FilesystemSource interface.
func (v *filesystemSource) ReleaseFilesystems(ctx context.Context, ids []string) ([]error, error) {
	// noop
	return make([]error, len(ids)), nil
}

// AttachFilesystems is specified on the jujustorage.FilesystemSource interface.
func (v *filesystemSource) AttachFilesystems(
	ctx context.Context,
	params []jujustorage.FilesystemAttachmentParams,
) ([]jujustorage.AttachFilesystemsResult, error) {
	results := make([]jujustorage.AttachFilesystemsResult, 0, len(params))
	for _, param := range params {
		var result jujustorage.AttachFilesystemsResult
		if param.AttachmentParams.ProviderId == nil {
			result.Error = errors.Errorf(
				"kubernetes filesystem %q attachment to %q missing provider id",
				param.Filesystem.Id(), param.Machine.Id(),
			).Add(jujustorage.FilesystemAttachParamsIncomplete)
			results = append(results, result)
			continue
		}
		// Kubernetes filesystems attachments are PersistentVolumeClaims
		// that are provisioned by the StatefulSet, not by the storage
		// provider. Instead we check that it exists and source any
		// filesystem attachment required information.
		info, err := v.getPersistentVolumeClaim(
			ctx,
			*param.AttachmentParams.ProviderId,
			param.Path,
			param.ReadOnly,
		)
		if err != nil {
			result.Error = errors.Errorf(
				"finalising kubernetes filesystem %q attachment to %q with PersistentVolumeClaim %q: %w",
				param.Filesystem.Id(),
				param.Machine.Id(),
				*param.AttachmentParams.ProviderId,
				err,
			)
		} else {
			result.FilesystemAttachment = &jujustorage.FilesystemAttachment{
				Filesystem:               param.Filesystem,
				Machine:                  param.Machine,
				FilesystemAttachmentInfo: info,
			}
		}

		results = append(results, result)
	}
	return results, nil
}

func (v *filesystemSource) getPersistentVolumeClaim(
	ctx context.Context, pvcName string, mountPath string, readOnly bool,
) (jujustorage.FilesystemAttachmentInfo, error) {
	client := v.client.client()
	pvcAPI := client.CoreV1().PersistentVolumeClaims(v.client.namespace)
	_, err := pvcAPI.Get(ctx, pvcName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return jujustorage.FilesystemAttachmentInfo{}, errors.New(
			"kubernetes PersistentVolumeClaim not found",
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return jujustorage.FilesystemAttachmentInfo{}, errors.Errorf(
			"getting kubernetes PersistentVolumeClaim: %w", err,
		)
	}
	// TODO(storage): it might be good to get the source of truth of these
	// values from the Pod, but might not be possible.
	fsAttachment := jujustorage.FilesystemAttachmentInfo{
		Path:     mountPath,
		ReadOnly: readOnly,
	}
	return fsAttachment, nil
}

// DetachFilesystems is specified on the jujustorage.FilesystemSource interface.
func (v *filesystemSource) DetachFilesystems(
	ctx context.Context,
	params []jujustorage.FilesystemAttachmentParams,
) ([]error, error) {
	pvcNamesInformational := make([]string, 0, len(params))
	for _, param := range params {
		if param.AttachmentParams.ProviderId != nil {
			pvcNamesInformational = append(
				pvcNamesInformational, *param.AttachmentParams.ProviderId)
		}
	}
	logger.Infof(
		ctx, "destroying kubernetes PersistentVolumeClaim(s): %v",
		pvcNamesInformational,
	)
	errs := make([]error, 0, len(params))
	for _, param := range params {
		if param.AttachmentParams.ProviderId == nil {
			err := errors.Errorf(
				"kubernetes filesystem %q attachment to %q missing provider id",
				param.Filesystem.Id(), param.Machine.Id(),
			)
			errs = append(errs, err)
			continue
		}
		pvcName := *param.AttachmentParams.ProviderId
		err := v.deletePersistentVolumeClaim(ctx, pvcName)
		if err != nil {
			err = errors.Errorf(
				"destroying kubernetes PersistentVolumeClaim %q: %w",
				pvcName, err,
			)
		}
		errs = append(errs, err)
	}
	return errs, nil
}

func (v *filesystemSource) deletePersistentVolumeClaim(
	ctx context.Context, pvcName string,
) error {
	client := v.client.client()
	pvcAPI := client.CoreV1().PersistentVolumeClaims(v.client.namespace)
	pvc, err := pvcAPI.Get(ctx, pvcName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Errorf(
			"getting kubernetes PersistentVolumeClaim: %w", err,
		)
	}
	if pvc.Spec.VolumeName != "" {
		err := v.ensurePersistentVolumeWillRetain(ctx, pvc.Spec.VolumeName)
		if err != nil {
			return errors.Errorf(
				"updating kubernetes PersistentVolume %q ReclaimPolicy to Retain",
				pvc.Spec.VolumeName,
			)
		}
	}
	err = pvcAPI.Delete(ctx, pvcName, v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Errorf(
			"deleting kubernetes PersistentVolumeClaim: %w", err,
		)
	}
	return nil
}

func (v *filesystemSource) ensurePersistentVolumeWillRetain(
	ctx context.Context, pvName string,
) error {
	pvAPI := v.client.client().CoreV1().PersistentVolumes()
	pv, err := pvAPI.Get(ctx, pvName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		logger.Warningf(
			ctx, "kubernetes PersistentVolume %q missing during PersistentVolumeClaim delete",
			pvName,
		)
		return nil
	} else if err != nil {
		return errors.Errorf("getting kubernetes PersistentVolume: %w", err)
	}
	if pv.Spec.PersistentVolumeReclaimPolicy == core.PersistentVolumeReclaimRetain {
		return nil
	}
	pv.Spec.PersistentVolumeReclaimPolicy = core.PersistentVolumeReclaimRetain
	pv, err = pvAPI.Update(ctx, pv, v1.UpdateOptions{
		FieldManager: "juju",
	})
	if k8serrors.IsNotFound(err) {
		logger.Warningf(
			ctx, "kubernetes PersistentVolume %q missing during Retain update",
			pvName,
		)
		return nil
	} else if err != nil {
		return errors.Errorf("updating kubernetes PersistentVolume: %w", err)
	}
	if pv.Spec.PersistentVolumeReclaimPolicy != core.PersistentVolumeReclaimRetain {
		return errors.Errorf(
			"kubernetes PersistentVolume %q ReclaimPolicy unable to be set to Retain",
			pvName,
		)
	}
	return nil
}

// ImportFilesystem is specified on the jujustorage.FilesystemImporter interface.
func (v *filesystemSource) ImportFilesystem(
	ctx context.Context,
	filesystemId string,
	resourceTags map[string]string,
) (jujustorage.FilesystemInfo, error) {
	return jujustorage.FilesystemInfo{}, errors.New("import filesystem not implemented")
}
