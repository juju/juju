# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2019 Canonical Ltd.
#
# This program is free software: you can redistribute it and/or modify it
# under the terms of the Lesser GNU General Public License version 3, as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful, but WITHOUT
# ANY WARRANTY; without even the implied warranties of MERCHANTABILITY,
# SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR PURPOSE.  See the Lesser
# GNU General Public License for more details.
#
# You should have received a copy of the Lesser GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

# Functionality for handling installed or other juju binaries
# (including paths etc.)


from __future__ import print_function

import logging
import tempfile
import subprocess

from .base import (
    Base,
    K8sProviderType,
)
from .factory import register_provider


logger = logging.getLogger(__name__)


LXD_PROFILE = """
name: juju-{model_name}
config:
  boot.autostart: "true"
  linux.kernel_modules: ip_tables,ip6_tables,netlink_diag,nf_nat,overlay
  raw.lxc: |
    lxc.apparmor.profile=unconfined
    lxc.mount.auto=proc:rw sys:rw
    lxc.cap.drop=
  security.nesting: "true"
  security.privileged: "true"
description: ""
devices:
  aadisable:
    path: /sys/module/nf_conntrack/parameters/hashsize
    source: /dev/null
    type: disk
  aadisable1:
    path: /sys/module/apparmor/parameters/enabled
    source: /dev/null
    type: disk
"""

BUNDLE = """
series: bionic
description: A minimal two-machine Kubernetes cluster, appropriate for development.
machines:
  '0':
    constraints: cores=2 mem=4G root-disk=16G
    series: bionic
  '1':
    constraints: cores=4 mem=8G root-disk=20G
    series: bionic
services:
  easyrsa:
    annotations:
      gui-x: '450'
      gui-y: '550'
    charm: cs:~containers/easyrsa-185
    num_units: 1
    to:
    - lxd:0
  etcd:
    annotations:
      gui-x: '800'
      gui-y: '550'
    charm: cs:~containers/etcd-319
    num_units: 1
    options:
      channel: 3.2/stable
    to:
    - '0'
  flannel:
    annotations:
      gui-x: '450'
      gui-y: '750'
    charm: cs:~containers/flannel-339
    resources:
      flannel-amd64: 3
      flannel-arm64: 1
      flannel-s390x: 3
  kubernetes-master:
    annotations:
      gui-x: '800'
      gui-y: '850'
    charm: cs:~containers/kubernetes-master-522
    constraints: cores=2 mem=4G root-disk=16G
    expose: true
    num_units: 1
    options:
      channel: 1.12/stable
    to:
    - '0'
  kubernetes-worker:
    annotations:
      gui-x: '100'
      gui-y: '850'
    charm: cs:~containers/kubernetes-worker-378
    constraints: cores=4 mem=8G root-disk=20G
    expose: true
    num_units: 1
    options:
      channel: 1.12/stable
      proxy-extra-args: proxy-mode=userspace
    to:
    - '1'
relations:
- - kubernetes-master:kube-api-endpoint
  - kubernetes-worker:kube-api-endpoint
- - kubernetes-master:kube-control
  - kubernetes-worker:kube-control
- - kubernetes-master:certificates
  - easyrsa:self.
- - kubernetes-master:etcd
  - etcd:db
- - kubernetes-worker:certificates
  - easyrsa:client
- - etcd:certificates
  - easyrsa:client
- - flannel:etcd
  - etcd:db
- - flannel:cni
  - kubernetes-master:cni
- - flannel:cni
  - kubernetes-worker:cni
"""

HOST_PATH_PROVISIONER = """
apiVersion: v1
kind: ServiceAccount
metadata:
  name: hostpath-provisioner
  namespace: kube-system
---

apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: hostpath-provisioner
  namespace: kube-system
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list", "watch", "create", "update", "patch"]
---

apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRoleBinding
metadata:
  name: hostpath-provisioner
  namespace: kube-system
subjects:
  - kind: ServiceAccount
    name: hostpath-provisioner
    namespace: kube-system
roleRef:
  kind: ClusterRole
  name: hostpath-provisioner
  apiGroup: rbac.authorization.k8s.io
---

apiVersion: rbac.authorization.k8s.io/v1beta1
kind: Role
metadata:
  name: hostpath-provisioner
  namespace: kube-system
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["create", "get", "delete"]
---

apiVersion: rbac.authorization.k8s.io/v1beta1
kind: RoleBinding
metadata:
  name: hostpath-provisioner
  namespace: kube-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: hostpath-provisioner
subjects:
  - kind: ServiceAccount
    name: hostpath-provisioner
---

# -- Create a daemon set for web requests and send them to the nginx-ingress-controller
apiVersion: extensions/v1beta1
kind: DaemonSet
metadata:
  name: hostpath-provisioner
  namespace: kube-system
spec:
  revisionHistoryLimit: 3
  template:
    metadata:
      labels:
        app: hostpath-provisioner
    spec:
      serviceAccountName: hostpath-provisioner
      terminationGracePeriodSeconds: 0
      containers:
        - name: hostpath-provisioner
          image: mazdermind/hostpath-provisioner:latest
          imagePullPolicy: "IfNotPresent"
          env:
            - name: NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
            - name: PV_DIR
              value: /mnt/kubernetes
          volumeMounts:
            - name: pv-volume
              mountPath: /mnt/kubernetes
      volumes:
        - name: pv-volume
          hostPath:
            path: /mnt/kubernetes
---

# -- Create the standard storage class for running on-node hostpath storage
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  # namespace: kube-system
  name: {class_name}
  annotations:
    storageclass.beta.kubernetes.io/is-default-class: "true"
  labels:
    kubernetes.io/cluster-service: "true"
    addonmanager.kubernetes.io/mode: EnsureExists
provisioner: hostpath
---
"""


@register_provider
class KubernetesCore(Base):

    name = K8sProviderType.K8S_CORE

    def __init__(self, client, timeout=1800):
        super().__init__(client, timeout)
        self.default_storage_class_name = "juju-storageclass"

    def _ensure_kube_dir(self):
        # ensure kube credentials
        self.client.juju('scp', ('kubernetes-master/0:config', self.kube_config_path))

        # ensure kubectl by scp from master
        self.client.juju('scp', ('kubernetes-master/0:/snap/kubectl/current/kubectl', self.kubectl_path))

    def _ensure_cluster_stack(self):
        self.__ensure_lxd_profile(
            LXD_PROFILE,
            self.client.model_name  # current model now is the default IAAS hosted model for hosting k8s cluster.
        )
        self.__deploy_stack_bundle(BUNDLE)

    def _ensure_cluster_config(self):
        self.__ensure_tmp_fix_for_ingress()
        self.__ensure_storage_provisoner_default_sc()

    def _node_address_getter(self, node):
        # TODO(ycliuhw): implement here once described k8s core node to find the corrent node ip.
        raise NotImplemented()

    def __ensure_lxd_profile(self, profile, model_name):
        profile = profile.format(model_name=model_name)
        with subprocess.Popen(('echo', profile), stdout=subprocess.PIPE) as echo:
            o = subprocess.check_output(
                ('lxc', 'profile', 'edit', 'juju-%s' % model_name),
                stdin=echo.stdout
            ).decode('UTF-8').strip()
            logger.debug(o)

    def __deploy_stack_bundle(self, bundle_content):
        with tempfile.NamedTemporaryFile() as f:
            f.write(bundle_content)
            self.client.deploy_bundle(f.name, static_bundle=True)

        # Wait for the deployment to finish.
        self.client.wait_for_started(timeout=self.timeout)

        # wait for cluster to stabilize
        self.client.wait_for_workloads(timeout=self.timeout)

        # get current status with tabular format for better debugging
        self.client.juju(self.client._show_status, ('--format', 'tabular'))

    def __ensure_tmp_fix_for_ingress(self):
        # tmp fix kubernetes core ingress issue
        ing_daemonset_name = 'daemonset.apps/nginx-ingress-kubernetes-worker-controller'
        o = self.kubectl(
            'patch', ing_daemonset_name, '--patch',
            '''
            {"spec": {"template": {"spec": {"containers": [{"name": "nginx-ingress-kubernetes-worker","args": ["/nginx-ingress-controller", "--default-backend-service=$(POD_NAMESPACE)/default-http-backend", "--configmap=$(POD_NAMESPACE)/nginx-load-balancer-conf", "--enable-ssl-chain-completion=False", "--publish-status-address=%s"]}]}}}}
            ''' % self.get_first_worker_ip()
        )
        logger.info(o)

        o = self.kubectl('get', ing_daemonset_name, '-o', 'yaml')
        logger.info(o)

    def __ensure_storage_provisoner_default_sc(self):
        # deploy hostpath storage provisioner and default storage class.
        self.client.kubectl_apply(HOST_PATH_PROVISIONER.format(class_name=self.default_storage_class_name))
