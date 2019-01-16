#!/usr/bin/env python3
""" Test caas k8s cluster bootstrap

    1. spinning up k8s cluster and asserting the cluster is `healthy`;
    2. deploy gitlab, mysql charms to caas model;
    3. relate gitlab mysql;
    4. assert http health check on gitlab
"""

from __future__ import print_function

import argparse
import logging
import sys
import os
import subprocess
from time import sleep

import requests

from deploy_stack import (
    BootstrapManager,
    deploy_caas_stack,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
)

from jujucharm import (
    local_charm_path
)
from jujupy.utility import until_timeout

__metaclass__ = type


log = logging.getLogger("assess_caas_charm_deployment")

JUJU_STORAGECLASS_NAME = "juju-storageclass"
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


def check_app_healthy(url, timeout=300):
    status_code = None
    for _ in until_timeout(timeout):
        try:
            r = requests.get(url)
            if r.ok and r.status_code < 400:
                return
            status_code = r.status_code
        except IOError as e:
            log.error(e)
        sleep(3)
    log.error('HTTP health check failed -> %s, status_code -> %s !', url, status_code)
    raise JujuAssertionError('gitlab is not healthy')


def assess_caas_charm_deployment(client):
    # Deploy k8s bundle to spin up k8s cluster
    bundle = local_charm_path(
        charm='bundles-kubernetes-core-lxd.yaml',
        repository=os.environ['JUJU_REPOSITORY'],
        juju_ver=client.version
    )

    caas_client = deploy_caas_stack(bundle_path=bundle, client=client)
    external_hostname = caas_client.get_external_hostname()

    if not caas_client.is_cluster_healthy:
        raise JujuAssertionError('k8s cluster is not healthy because kubectl is not accessible')

    # tmp fix kubernetes core ingress issue
    caas_client.kubectl(
        'patch', 'daemonset.apps/nginx-ingress-kubernetes-worker-controller', '--patch',
        '''
        {"spec": {"template": {"spec": {"containers": [{"name": "nginx-ingress-kubernetes-worker","args": ["/nginx-ingress-controller", "--default-backend-service=$(POD_NAMESPACE)/default-http-backend", "--configmap=$(POD_NAMESPACE)/nginx-load-balancer-conf", "--enable-ssl-chain-completion=False", "--publish-status-address=%s"]}]}}}}
        ''' % caas_client.get_first_worker_ip()
    )

    # add caas model for deploying caas charms on top of it
    model_name = 'testcaas'
    k8s_model = caas_client.add_model(model_name)

    # ensure tmp dir for storage class.model_name
    o = subprocess.check_output(
        ('sudo', 'mkdir', '-p', '/mnt/kubernetes/%s' % model_name)  # unfortunately, needs sudo
    )
    log.debug(o.decode('UTF-8').strip())

    # ensure storage class
    caas_client.kubectl_apply(HOST_PATH_PROVISIONER.format(class_name=JUJU_STORAGECLASS_NAME))

    # ensure storage pools for caas operator
    k8s_model.juju(
        'create-storage-pool',
        ('operator-storage', 'kubernetes', 'storage-class=%s' % JUJU_STORAGECLASS_NAME)
    )

    # ensure storage pools for mariadb
    mariadb_storage_pool_name = 'mariadb-pv'
    k8s_model.juju(
        'create-storage-pool',
        (mariadb_storage_pool_name, 'kubernetes', 'storage-class=%s' % JUJU_STORAGECLASS_NAME)
    )

    k8s_model.deploy(
        charm="cs:~juju/gitlab-k8s-0",
        config='juju-external-hostname={}'.format(external_hostname),
    )

    k8s_model.deploy(
        charm="cs:~juju/mariadb-k8s-0",
        storage='database=100M,{pool_name}'.format(pool_name=mariadb_storage_pool_name),
    )

    k8s_model.juju('relate', ('mariadb-k8s', 'gitlab-k8s'))
    k8s_model.juju('expose', ('gitlab-k8s',))
    k8s_model.wait_for_workloads(timeout=3600)

    url = '{}://{}/{}'.format('http', external_hostname, 'gitlab-k8s')
    check_app_healthy(url, timeout=1200)

    log.info(caas_client.kubectl('get', 'all', '--all-namespaces'))
    k8s_model.juju(k8s_model._show_status, ('--format', 'tabular'))


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Cass charm deployment CI test")
    parser.add_argument(
        '--caas-image', action='store', default=None,
        help='Caas operator docker image name to use with format of <username>/caas-jujud-operator:<tag>.'
    )

    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)


def ensure_operator_image_path(client, image_path):
    client.controller_juju('controller-config', ('caas-operator-image-path={}'.format(image_path),))


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        client = bs_manager.client
        ensure_operator_image_path(client, image_path=args.caas_image)
        assess_caas_charm_deployment(client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
