// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	k8sutils "github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/uuid"
)

// ClientConfigFuncGetter returns a function returning az reader that will read a k8s cluster config for a given cluster type
type ClientConfigFuncGetter func(string) (clientconfig.ClientConfigFunc, error)

// GetClusterMetadataFunc returns the ClusterMetadata using the provided ClusterMetadataChecker
type GetClusterMetadataFunc func(KubeCloudStorageParams) (*k8s.ClusterMetadata, error)

// KubeCloudParams defines the parameters used to extract a k8s cluster definition from kubeconfig data.
type KubeCloudParams struct {
	ClusterName string
	ContextName string
	CloudName   string
	// CredentialUID ensures RBAC resources are unique.
	CredentialUID      string
	HostCloudRegion    string
	CaasType           string
	ClientConfigGetter ClientConfigFuncGetter
	Clock              jujuclock.Clock
}

// KubeCloudStorageParams defines the parameters used to determine storage details for a k8s cluster.
type KubeCloudStorageParams struct {
	WorkloadStorage        string
	HostCloudRegion        string
	MetadataChecker        k8s.ClusterMetadataChecker
	GetClusterMetadataFunc GetClusterMetadataFunc
}

// UpdateKubeCloudWithStorage updates the passed Cloud with storage details retrieved from the cloud's cluster.
// On error the partially updated cloud is still returned so that callers
// able to tolerate the error (see FinalizeCloud) can retain the updates
// that were made.
func UpdateKubeCloudWithStorage(k8sCloud cloud.Cloud, storageParams KubeCloudStorageParams) (cloud.Cloud, error) {
	// Get the cluster metadata and see what storage comes back based on the
	// preffered rules for metadata.
	clusterMetadata, err := storageParams.GetClusterMetadataFunc(storageParams)
	if err != nil {
		return k8sCloud, ClusterQueryError{Message: err.Error(), Err: err}
	}
	if clusterMetadata == nil {
		return k8sCloud, ClusterQueryError{Message: "cannot get cluster metadata"}
	}

	if storageParams.HostCloudRegion == "" && clusterMetadata.Cloud != "" {
		var region string
		if clusterMetadata.Regions != nil && clusterMetadata.Regions.Size() > 0 {
			region = clusterMetadata.Regions.SortedValues()[0]
		}
		storageParams.HostCloudRegion = cloud.BuildHostCloudRegion(clusterMetadata.Cloud, region)
	}
	k8sCloud.HostCloudRegion = storageParams.HostCloudRegion

	if k8sCloud.HostCloudRegion != "" {
		_, region, err := cloud.SplitHostCloudRegion(k8sCloud.HostCloudRegion)
		if err != nil {
			// Shouldn't happen as HostCloudRegion is validated earlier.
			return k8sCloud, errors.Trace(err)
		}
		if region != "" {
			k8sCloud.Regions = []cloud.Region{{
				Name:     region,
				Endpoint: k8sCloud.Endpoint,
			}}
		}
	}

	if k8sCloud.Config == nil {
		k8sCloud.Config = make(map[string]any)
	}

	if clusterMetadata.WorkloadStorageClass != nil {
		// Record the storage class on the cloud so that bootstrap can
		// create the default storage pool for the controller model from
		// it, and so that subsequent add-k8s and bootstrap calls retain
		// and re-validate the choice.
		k8sCloud.Config[constants.WorkloadStorageKey] =
			clusterMetadata.WorkloadStorageClass.Name
		return k8sCloud, nil
	}

	// The controller stack always creates a persistent volume claim, so a
	// workload storage class is required to bootstrap. Let the caller
	// decide how to report this: add-k8s errors out while bootstrap's
	// FinalizeCloud tolerates it, because the controller storage class can
	// still be resolved at bootstrap time from the storage pools scheduled
	// for the controller model.
	return k8sCloud, &environs.PreferredStorageNotFound{
		Message: "no preferred workload storage class found in Kubernetes cluster",
	}
}

// BaseKubeCloudOpenParams provides a basic OpenParams for a cluster
func BaseKubeCloudOpenParams(cloud cloud.Cloud, credential cloud.Credential) (environs.OpenParams, error) {
	// To get a k8s client, we need a config with minimal information.
	// It's not used unless operating on a real model but we need to supply it.
	modelUUID, err := uuid.NewUUID()
	if err != nil {
		return environs.OpenParams{}, errors.Trace(err)
	}
	controllerUUID, err := uuid.NewUUID()
	if err != nil {
		return environs.OpenParams{}, errors.Trace(err)
	}
	attrs := map[string]any{
		config.NameKey: "add-cloud",
		config.TypeKey: "kubernetes",
		config.UUIDKey: modelUUID.String(),
	}
	cfg, err := config.New(config.UseDefaults, attrs)
	if err != nil {
		return environs.OpenParams{}, errors.Trace(err)
	}

	cloudSpec, err := environscloudspec.MakeCloudSpec(cloud, "", &credential)
	if err != nil {
		return environs.OpenParams{}, errors.Trace(err)
	}
	openParams := environs.OpenParams{
		ControllerUUID: controllerUUID.String(),
		Cloud:          cloudSpec,
		Config:         cfg,
	}
	return openParams, nil
}

// FinalizeCloud is part of the environs.CloudFinalizer interface.
func (p kubernetesEnvironProvider) FinalizeCloud(ctx environs.FinalizeCloudContext, cld cloud.Cloud) (cloud.Cloud, error) {
	// We set the clouds auth types to all kubernetes supported auth types here
	// so that finalize credentials is free to change the credentials of the
	// bootstrap. See lp-1918486
	cld.AuthTypes = k8scloud.SupportedAuthTypes()

	var credentials cloud.Credential
	if cld.Name != k8s.K8sCloudMicrok8s {
		creds, err := p.RegisterCredentials(cld)
		if err != nil {
			return cld, err
		}

		cloudCred, exists := creds[cld.Name]
		if !exists {
			return cld, nil
		}

		credentials = cloudCred.AuthCredentials[creds[cld.Name].DefaultCredential]
	} else {
		// Need the credentials, need to query for those details
		mk8sCloud, err := p.builtinCloudGetter(p.cmdRunner)
		if err != nil {
			return cloud.Cloud{}, errors.Trace(err)
		}
		cld = mk8sCloud

		creds, err := p.RegisterCredentials(cld)
		if err != nil {
			return cld, err
		}

		credentials = creds[cld.Name].AuthCredentials[creds[cld.Name].DefaultCredential]
	}

	if cld.SkipTLSVerify {
		logger.Warningf(context.TODO(), "k8s cloud %v is configured to skip server certificate validity checks", cld.Name)
	}

	openParams, err := BaseKubeCloudOpenParams(cld, credentials)
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	broker, err := p.brokerGetter(ctx, openParams, environs.NoopCredentialInvalidator())
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	if cld.Name == k8s.K8sCloudMicrok8s {
		if err := ensureMicroK8sSuitable(ctx, broker); err != nil {
			return cld, errors.Trace(err)
		}
	}
	// A workload storage class recorded on the cloud (by add-k8s) is
	// re-validated here so that a stale or missing choice is surfaced
	// early. There is nothing to nominate for the built-in microk8s
	// cloud, whose storage is validated by ensureMicroK8sSuitable above.
	nominatedStorageClass, _ := cld.Config[constants.WorkloadStorageKey].(string)
	storageUpdateParams := KubeCloudStorageParams{
		MetadataChecker: broker,
		GetClusterMetadataFunc: func(storageParams KubeCloudStorageParams) (*k8s.ClusterMetadata, error) {
			clusterMetadata, err := broker.GetClusterMetadata(ctx, nominatedStorageClass)
			if err != nil {
				var notFoundNominated *environs.NominatedStorageNotFound
				if nominatedStorageClass != "" && errors.As(err, &notFoundNominated) {
					// The class recorded on the cloud no longer exists on
					// the cluster. Fall back to discovery rather than
					// failing bootstrap here, keeping the stale recorded
					// class so that a later unresolvable bootstrap failure
					// names the class that needs fixing.
					logger.Warningf(ctx,
						"workload storage class %q recorded on k8s cloud %q not found on the cluster",
						nominatedStorageClass, cld.Name)
					clusterMetadata, err = broker.GetClusterMetadata(ctx, "")
				}
			}
			if err != nil {
				return nil, errors.Trace(err)
			}
			return clusterMetadata, nil
		},
	}

	updatedCloud, err := UpdateKubeCloudWithStorage(cld, storageUpdateParams)
	if err != nil {
		// add-k8s reports a missing workload storage class as an error, but
		// FinalizeCloud tolerates it: bootstrap can still resolve a
		// controller storage class from the storage pools scheduled for
		// the controller model, and an unresolvable bootstrap fails later
		// with a more specific message. The partially updated cloud is
		// kept, so any updates made before the failure are not lost.
		var notFoundPreferred *environs.PreferredStorageNotFound
		if !errors.As(err, &notFoundPreferred) {
			return cld, errors.Trace(err)
		}
		logger.Warningf(ctx,
			"unable to determine a workload storage class for k8s cloud %q: %v",
			cld.Name, err)
	}
	cld = updatedCloud

	if cld.HostCloudRegion == "" {
		cld.HostCloudRegion = k8s.K8sCloudOther
	}

	return cld, nil
}

func checkDefaultStorageExist(ctx context.Context, broker ClusterMetadataStorageChecker) error {
	storageClasses, err := broker.ListStorageClasses(ctx, k8slabels.NewSelector())
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Annotate(err, "cannot list storage classes")
	}
	for _, sc := range storageClasses {
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			return nil
		}
	}
	return errors.NotFoundf("default storage")
}

func checkDNSAddonEnabled(ctx context.Context, broker ClusterMetadataStorageChecker) error {
	pods, err := broker.ListPods(ctx, "kube-system", k8sutils.LabelsToSelector(map[string]string{"k8s-app": "kube-dns"}))
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Annotate(err, "cannot list kube-dns pods")
	}
	if len(pods) > 0 {
		return nil
	}
	return errors.NotFoundf("dns pod")
}

func ensureMicroK8sSuitable(ctx context.Context, broker ClusterMetadataStorageChecker) error {
	err := checkDefaultStorageExist(ctx, broker)
	if errors.Is(err, errors.NotFound) {
		return errors.New("required storage addon is not enabled")
	}
	if err != nil {
		return errors.Trace(err)
	}

	err = checkDNSAddonEnabled(ctx, broker)
	if errors.Is(err, errors.NotFound) {
		return errors.New("required dns addon is not enabled")
	}
	return errors.Trace(err)
}
