// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"reflect"
	"sort"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

// AppNameForServiceAccount returns the juju application name associated with a
// given ServiceAccount. If app name cannot be obtained from the service
// account, errors.NotFound is returned.
func AppNameForServiceAccount(sa *core.ServiceAccount) (string, error) {
	if appName, ok := sa.Labels[constants.LabelKubernetesAppName]; ok {
		return appName, nil
	} else if appName, ok := sa.Labels[constants.LegacyLabelKubernetesAppName]; ok {
		return appName, nil
	}
	return "", errors.NotFoundf("application labels for service account %s", sa.Name)
}

// RBACLabels returns a set of labels that should be present for RBAC objects.
func RBACLabels(appName, modelName, modelUUID, controllerUUID string, global bool, labelVersion constants.LabelVersion) map[string]string {
	labels := utils.LabelsForApp(appName, labelVersion)
	if global {
		labels = utils.LabelsMerge(labels, utils.LabelsForModel(modelName, modelUUID, controllerUUID, labelVersion))
	}
	return labels
}

func (k *kubernetesClient) createServiceAccount(ctx context.Context, sa *core.ServiceAccount) (*core.ServiceAccount, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(sa)
	out, err := k.client().CoreV1().ServiceAccounts(k.namespace).Create(ctx, sa, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("service account %q", sa.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateServiceAccount(ctx context.Context, sa *core.ServiceAccount) (*core.ServiceAccount, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().CoreV1().ServiceAccounts(k.namespace).Update(ctx, sa, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("service account %q", sa.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) ensureServiceAccount(ctx context.Context, sa *core.ServiceAccount) (out *core.ServiceAccount, cleanups []func(), err error) {
	out, err = k.createServiceAccount(ctx, sa)
	if err == nil {
		logger.Debugf(ctx, "service account %q created", out.GetName())
		cleanups = append(cleanups, func() { _ = k.deleteServiceAccount(ctx, out.GetName(), out.GetUID()) })
		return out, cleanups, nil
	}
	if !errors.IsAlreadyExists(err) {
		return nil, cleanups, errors.Trace(err)
	}

	existing, err := k.getServiceAccount(ctx, sa.GetName())
	if err != nil {
		return nil, cleanups, errors.Trace(err)
	}

	var existingLabelVersion constants.LabelVersion
	switch name := sa.GetName(); name {
	case ExecRBACResourceName, modelOperatorName:
		existingLabelVersion, err = utils.MatchOperatorMetaLabelVersion(existing.ObjectMeta, modelOperatorName, OperatorModelTarget)
	default:
		existingLabelVersion, err = utils.MatchApplicationMetaLabelVersion(existing.ObjectMeta, name)
	}
	if err != nil {
		return nil, cleanups, errors.Annotatef(err, "ensuring ServiceAccount %q with labels %v ", sa.GetName(), existing.Labels)
	}
	if existingLabelVersion < k.labelVersion {
		logger.Warningf(ctx, "updating label version for existing ServiceAccount %q from %d to %d ", sa.GetName(), existingLabelVersion, k.labelVersion)
	}

	out, err = k.updateServiceAccount(ctx, sa)
	logger.Debugf(ctx, "updating service account %q", sa.GetName())
	return out, cleanups, errors.Trace(err)
}

func (k *kubernetesClient) getServiceAccount(ctx context.Context, name string) (*core.ServiceAccount, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().CoreV1().ServiceAccounts(k.namespace).Get(ctx, name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("service account %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteServiceAccount(ctx context.Context, name string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().CoreV1().ServiceAccounts(k.namespace).Delete(ctx, name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) createRole(ctx context.Context, role *rbacv1.Role) (*rbacv1.Role, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(role)
	out, err := k.client().RbacV1().Roles(k.namespace).Create(ctx, role, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("role %q", role.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateRole(ctx context.Context, role *rbacv1.Role) (*rbacv1.Role, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().RbacV1().Roles(k.namespace).Update(ctx, role, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("role %q", role.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) ensureRole(ctx context.Context, role *rbacv1.Role) (out *rbacv1.Role, cleanups []func(), err error) {
	out, err = k.createRole(ctx, role)
	if err == nil {
		logger.Debugf(ctx, "role %q created", out.GetName())
		cleanups = append(cleanups, func() { _ = k.deleteRole(ctx, out.GetName(), out.GetUID()) })
		return out, cleanups, nil
	}
	if !errors.Is(err, errors.AlreadyExists) {
		return nil, cleanups, errors.Trace(err)
	}

	existing, err := k.getRole(ctx, role.GetName())
	if err != nil {
		return nil, cleanups, errors.Trace(err)
	}

	var existingLabelVersion constants.LabelVersion
	switch name := role.GetName(); name {
	case ExecRBACResourceName, modelOperatorName:
		existingLabelVersion, err = utils.MatchOperatorMetaLabelVersion(existing.ObjectMeta, modelOperatorName, OperatorModelTarget)
	default:
		existingLabelVersion, err = utils.MatchApplicationMetaLabelVersion(existing.ObjectMeta, name)
	}
	if err != nil {
		return nil, cleanups, errors.Annotatef(err, "ensuring Role %q with labels %v ", role.GetName(), existing.Labels)
	}
	if existingLabelVersion < k.labelVersion {
		logger.Warningf(ctx, "updating label version for existing Role %q from %d to %d ", role.GetName(), existingLabelVersion, k.labelVersion)
	}

	out, err = k.updateRole(ctx, role)
	logger.Debugf(ctx, "updating role %q", role.GetName())
	return out, cleanups, errors.Trace(err)
}

func (k *kubernetesClient) getRole(ctx context.Context, name string) (*rbacv1.Role, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().RbacV1().Roles(k.namespace).Get(ctx, name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("role %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteRole(ctx context.Context, name string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().RbacV1().Roles(k.namespace).Delete(ctx, name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteClusterRoles(ctx context.Context, selector k8slabels.Selector) error {
	err := k.client().RbacV1().ClusterRoles().DeleteCollection(ctx, v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, v1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listClusterRoles(ctx context.Context, selector k8slabels.Selector) ([]rbacv1.ClusterRole, error) {
	listOps := v1.ListOptions{
		LabelSelector: selector.String(),
	}
	cRList, err := k.client().RbacV1().ClusterRoles().List(ctx, listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(cRList.Items) == 0 {
		return nil, errors.NotFoundf("cluster role with selector %q", selector)
	}
	return cRList.Items, nil
}

func (k *kubernetesClient) createRoleBinding(ctx context.Context, rb *rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(rb)
	out, err := k.client().RbacV1().RoleBindings(k.namespace).Create(ctx, rb, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("role binding %q", rb.GetName())
	}
	return out, errors.Trace(err)
}

func ensureResourceDeleted(clock jujuclock.Clock, getResource func() error) error {
	notReadyYetErr := errors.New("resource is still being deleted")
	deletionChecker := func() error {
		err := getResource()
		if errors.Is(err, errors.NotFound) {
			return nil
		}
		if err == nil {
			return notReadyYetErr
		}
		return errors.Trace(err)
	}

	err := retry.Call(retry.CallArgs{
		Attempts: 10,
		Delay:    2 * time.Second,
		Clock:    clock,
		Func:     deletionChecker,
		IsFatalError: func(err error) bool {
			return err != nil && err != notReadyYetErr
		},
		NotifyFunc: func(error, int) {
			logger.Debugf(context.TODO(), "waiting for resource to be deleted")
		},
	})
	return errors.Trace(err)
}

func isRoleBindingEqual(a, b rbacv1.RoleBinding) bool {
	sortSubjects := func(s []rbacv1.Subject) {
		sort.Slice(s, func(i, j int) bool {
			return s[i].Name+s[i].Namespace+s[i].Kind > s[j].Name+s[j].Namespace+s[j].Kind
		})
	}
	sortSubjects(a.Subjects)
	sortSubjects(b.Subjects)

	// We don't compare labels.
	return reflect.DeepEqual(a.RoleRef, b.RoleRef) &&
		reflect.DeepEqual(a.Subjects, b.Subjects) &&
		reflect.DeepEqual(a.ObjectMeta.Annotations, b.ObjectMeta.Annotations)
}

func (k *kubernetesClient) ensureRoleBinding(ctx context.Context, rb *rbacv1.RoleBinding) (out *rbacv1.RoleBinding, cleanups []func(), err error) {
	isFirstDeploy := false
	// RoleRef is immutable, so delete first then re-create.
	existing, err := k.getRoleBinding(ctx, rb.GetName())
	if errors.Is(err, errors.NotFound) {
		isFirstDeploy = true
	} else if err != nil {
		return nil, cleanups, errors.Trace(err)
	}
	if existing != nil {
		if isRoleBindingEqual(*existing, *rb) {
			return existing, cleanups, nil
		}
		name := existing.GetName()
		uid := existing.GetUID()
		if err := k.deleteRoleBinding(ctx, name, uid); err != nil {
			return nil, cleanups, errors.Trace(err)
		}

		if err := ensureResourceDeleted(
			k.clock,
			func() error {
				_, err := k.getRoleBinding(ctx, name)
				return errors.Trace(err)
			},
		); err != nil {
			return nil, cleanups, errors.Trace(err)
		}
	}
	out, err = k.createRoleBinding(ctx, rb)
	if err != nil {
		return nil, cleanups, errors.Trace(err)
	}
	if isFirstDeploy {
		// only do cleanup for the first time, don't do this for existing deployments.
		cleanups = append(cleanups, func() { _ = k.deleteRoleBinding(ctx, out.GetName(), out.GetUID()) })
	}
	logger.Debugf(ctx, "role binding %q created", rb.GetName())
	return out, cleanups, nil
}

func (k *kubernetesClient) getRoleBinding(ctx context.Context, name string) (*rbacv1.RoleBinding, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().RbacV1().RoleBindings(k.namespace).Get(ctx, name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("role binding %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteRoleBinding(ctx context.Context, name string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().RbacV1().RoleBindings(k.namespace).Delete(ctx, name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteClusterRoleBindings(ctx context.Context, selector k8slabels.Selector) error {
	err := k.client().RbacV1().ClusterRoleBindings().DeleteCollection(ctx, v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, v1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listClusterRoleBindings(ctx context.Context, selector k8slabels.Selector) ([]rbacv1.ClusterRoleBinding, error) {
	listOps := v1.ListOptions{
		LabelSelector: selector.String(),
	}
	cRBList, err := k.client().RbacV1().ClusterRoleBindings().List(ctx, listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(cRBList.Items) == 0 {
		return nil, errors.NotFoundf("cluster role binding with selector %q", selector)
	}
	return cRBList.Items, nil
}
