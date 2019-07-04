// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas"
)

func (k *kubernetesClient) ensureServiceAccountForApp(appName string, caasSpec *caas.ServiceAccountSpec) (cleanups []func(), err error) {
	labels := map[string]string{labelApplication: appName}
	saSpec := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      caasSpec.Name,
			Namespace: k.namespace,
			Labels:    labels,
		},
		AutomountServiceAccountToken: caasSpec.AutomountServiceAccountToken,
	}
	// ensure service account;
	sa, err := k.updateServiceAccount(saSpec)
	if errors.IsNotFound(err) {
		if sa, err = k.createServiceAccount(saSpec); err != nil {
			return cleanups, errors.Trace(err)
		}
		cleanups = append(cleanups, func() { k.deleteServiceAccount(sa.GetName()) })
	}

	roleSpec := caasSpec.Capabilities.Role
	logger.Criticalf("roleSpec -> \n%+v", roleSpec)
	var roleRef rbacv1.RoleRef
	// no rule specified, reference to existing Role/ClusterRole.
	switch roleSpec.Type {
	case caas.Role:
		var r *rbacv1.Role
		if len(roleSpec.Rules) == 0 {
			r, err = k.getRole(roleSpec.Name)
			if err != nil {
				return cleanups, errors.Trace(err)
			}
		} else {
			// create or update Role/ClusterRole.
			r, err = k.ensureRole(&rbacv1.Role{
				ObjectMeta: v1.ObjectMeta{
					Name:      roleSpec.Name,
					Namespace: k.namespace,
					Labels:    labels,
				},
				Rules: roleSpec.Rules,
			})
			if err != nil {
				return cleanups, errors.Trace(err)
			}
		}
		roleRef = rbacv1.RoleRef{
			Name: r.GetName(),
			Kind: r.Kind,
		}
	case caas.ClusterRole:
		var cr *rbacv1.ClusterRole
		if len(roleSpec.Rules) == 0 {
			cr, err = k.getClusterRole(roleSpec.Name)
			if err != nil {
				return cleanups, errors.Trace(err)
			}
		} else {
			// create or update Role/ClusterRole.
			cr, err = k.ensureClusterRole(&rbacv1.ClusterRole{
				ObjectMeta: v1.ObjectMeta{
					Name:      roleSpec.Name,
					Namespace: k.namespace,
					Labels:    labels,
				},
				Rules: roleSpec.Rules,
			})
			if err != nil {
				return cleanups, errors.Trace(err)
			}
		}
		logger.Criticalf("cr --> %+v", cr)
		roleRef = rbacv1.RoleRef{
			Name: cr.GetName(),
			// TOOD: why cr.Kind == ""
			// Kind: cr.Kind,
			Kind: string(roleSpec.Type),
		}
	default:
		// this should never happen.
		return cleanups, errors.New("unsupported Role type")
	}
	logger.Criticalf("roleRef --> %+v", roleRef)

	rbSpec := caasSpec.Capabilities.RoleBinding
	logger.Criticalf("rbSpec -> \n%+v", rbSpec)
	roleBindingMeta := v1.ObjectMeta{
		Name:      rbSpec.Name,
		Namespace: sa.GetNamespace(),
		Labels:    labels,
	}
	roleBindingSASubject := rbacv1.Subject{
		Kind:      rbacv1.ServiceAccountKind,
		Name:      sa.GetName(),
		Namespace: sa.GetNamespace(),
	}
	switch rbSpec.Type {
	case caas.RoleBinding:
		_, err = k.ensureRoleBinding(&rbacv1.RoleBinding{
			ObjectMeta: roleBindingMeta,
			RoleRef:    roleRef,
			Subjects:   []rbacv1.Subject{roleBindingSASubject},
		})
	case caas.ClusterRoleBinding:
		_, err = k.ensureClusterRoleBinding(&rbacv1.ClusterRoleBinding{
			ObjectMeta: roleBindingMeta,
			RoleRef:    roleRef,
			Subjects:   []rbacv1.Subject{roleBindingSASubject},
		})
	default:
		// this should never happen.
		return cleanups, errors.New("unsupported Role binding type")
	}
	return cleanups, errors.Trace(err)
}

func (k *kubernetesClient) createServiceAccount(sa *core.ServiceAccount) (*core.ServiceAccount, error) {
	out, err := k.client().CoreV1().ServiceAccounts(k.namespace).Create(sa)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("service account %q", sa.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateServiceAccount(sa *core.ServiceAccount) (*core.ServiceAccount, error) {
	out, err := k.client().CoreV1().ServiceAccounts(k.namespace).Update(sa)
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("service account %q", sa.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) ensureServiceAccount(sa *core.ServiceAccount) (*core.ServiceAccount, error) {
	out, err := k.updateServiceAccount(sa)
	if errors.IsNotFound(err) {
		out, err = k.createServiceAccount(sa)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getServiceAccount(name string) (*core.ServiceAccount, error) {
	out, err := k.client().CoreV1().ServiceAccounts(k.namespace).Get(name, v1.GetOptions{IncludeUninitialized: true})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("service account %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteServiceAccount(name string) error {
	err := k.client().CoreV1().ServiceAccounts(k.namespace).Delete(name, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteServiceAccountsRolesBindings(appName string) error {
	listOps := v1.ListOptions{
		LabelSelector: applicationSelector(appName),
	}
	rbList, err := k.client().RbacV1().RoleBindings(k.namespace).List(listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range rbList.Items {
		if err := k.deleteRoleBinding(s.GetName()); err != nil {
			return errors.Trace(err)
		}
	}

	crbList, err := k.client().RbacV1().ClusterRoleBindings().List(listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range crbList.Items {
		if err := k.deleteClusterRoleBinding(s.GetName()); err != nil {
			return errors.Trace(err)
		}
	}

	rList, err := k.client().RbacV1().Roles(k.namespace).List(listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range rList.Items {
		if err := k.deleteRole(s.GetName()); err != nil {
			return errors.Trace(err)
		}
	}

	crList, err := k.client().RbacV1().ClusterRoles().List(listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range crList.Items {
		if err := k.deleteClusterRole(s.GetName()); err != nil {
			return errors.Trace(err)
		}
	}

	saList, err := k.client().CoreV1().ServiceAccounts(k.namespace).List(listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range saList.Items {
		if err := k.deleteServiceAccount(s.GetName()); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (k *kubernetesClient) createRole(role *rbacv1.Role) (*rbacv1.Role, error) {
	out, err := k.client().RbacV1().Roles(k.namespace).Create(role)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("role %q", role.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateRole(role *rbacv1.Role) (*rbacv1.Role, error) {
	out, err := k.client().RbacV1().Roles(k.namespace).Update(role)
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("role %q", role.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) ensureRole(role *rbacv1.Role) (*rbacv1.Role, error) {
	out, err := k.updateRole(role)
	if errors.IsNotFound(err) {
		out, err = k.createRole(role)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getRole(name string) (*rbacv1.Role, error) {
	out, err := k.client().RbacV1().Roles(k.namespace).Get(name, v1.GetOptions{IncludeUninitialized: true})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("role %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteRole(name string) error {
	err := k.client().RbacV1().Roles(k.namespace).Delete(name, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteRoles(appName string) error {
	roleList, err := k.client().RbacV1().Roles(k.namespace).List(v1.ListOptions{
		LabelSelector: applicationSelector(appName),
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range roleList.Items {
		if err := k.deleteRole(s.GetName()); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (k *kubernetesClient) createClusterRole(cRole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	out, err := k.client().RbacV1().ClusterRoles().Create(cRole)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("cluster role %q", cRole.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateClusterRole(cRole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	out, err := k.client().RbacV1().ClusterRoles().Update(cRole)
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("cluster role %q", cRole.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) ensureClusterRole(cRole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	out, err := k.updateClusterRole(cRole)
	if errors.IsNotFound(err) {
		out, err = k.createClusterRole(cRole)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getClusterRole(name string) (*rbacv1.ClusterRole, error) {
	out, err := k.client().RbacV1().ClusterRoles().Get(name, v1.GetOptions{IncludeUninitialized: true})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("cluster role %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteClusterRole(name string) error {
	err := k.client().RbacV1().ClusterRoles().Delete(name, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteClusterRoles(appName string) error {
	clusterRoleList, err := k.client().RbacV1().ClusterRoles().List(v1.ListOptions{
		LabelSelector: applicationSelector(appName),
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range clusterRoleList.Items {
		if err := k.deleteClusterRole(s.GetName()); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (k *kubernetesClient) createRoleBinding(rb *rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	out, err := k.client().RbacV1().RoleBindings(k.namespace).Create(rb)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("role binding %q", rb.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateRoleBinding(rb *rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	out, err := k.client().RbacV1().RoleBindings(k.namespace).Update(rb)
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("role binding %q", rb.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) ensureRoleBinding(rb *rbacv1.RoleBinding) (*rbacv1.RoleBinding, error) {
	out, err := k.updateRoleBinding(rb)
	if errors.IsNotFound(err) {
		out, err = k.createRoleBinding(rb)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getRoleBinding(name string) (*rbacv1.RoleBinding, error) {
	out, err := k.client().RbacV1().RoleBindings(k.namespace).Get(name, v1.GetOptions{IncludeUninitialized: true})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("role binding %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteRoleBinding(name string) error {
	err := k.client().RbacV1().RoleBindings(k.namespace).Delete(name, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteRoleBindings(appName string) error {
	roleBindingList, err := k.client().RbacV1().RoleBindings(k.namespace).List(v1.ListOptions{
		LabelSelector: applicationSelector(appName),
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range roleBindingList.Items {
		if err := k.deleteRoleBinding(s.GetName()); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (k *kubernetesClient) createClusterRoleBinding(crb *rbacv1.ClusterRoleBinding) (*rbacv1.ClusterRoleBinding, error) {
	out, err := k.client().RbacV1().ClusterRoleBindings().Create(crb)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("cluster role binding %q", crb.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateClusterRoleBinding(crb *rbacv1.ClusterRoleBinding) (*rbacv1.ClusterRoleBinding, error) {
	out, err := k.client().RbacV1().ClusterRoleBindings().Update(crb)
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("cluster role binding %q", crb.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) ensureClusterRoleBinding(crb *rbacv1.ClusterRoleBinding) (*rbacv1.ClusterRoleBinding, error) {
	out, err := k.updateClusterRoleBinding(crb)
	if errors.IsNotFound(err) {
		out, err = k.createClusterRoleBinding(crb)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getClusterRoleBinding(name string) (*rbacv1.ClusterRoleBinding, error) {
	out, err := k.client().RbacV1().ClusterRoleBindings().Get(name, v1.GetOptions{IncludeUninitialized: true})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("cluster role binding %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteClusterRoleBinding(name string) error {
	err := k.client().RbacV1().ClusterRoleBindings().Delete(name, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteClusterRoleBindings(appName string) error {
	clusterRoleBindingList, err := k.client().RbacV1().ClusterRoleBindings().List(v1.ListOptions{
		LabelSelector: applicationSelector(appName),
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range clusterRoleBindingList.Items {
		if err := k.deleteClusterRole(s.GetName()); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
