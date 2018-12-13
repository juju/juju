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
JUJU_STORAGECLASS_TEMPLATE = """
kind: PersistentVolume
apiVersion: v1
metadata:
  name: {model}-data
spec:
  capacity:
    storage: 100Mi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  storageClassName: {class_name}
  hostPath:
    path: "/mnt/data/{model}"
"""


def check_app_healthy(url, timeout=300):
    status_code = None
    for _ in until_timeout(timeout):
        try:
            r = requests.get(url)
            if r.ok and r.status_code < 300:
                return
            status_code = r.status_code
        except IOError as e:
            log.error(e)
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

    # add caas model for deploying caas charms on top of it
    model_name = 'testcaas'
    k8s_model = caas_client.add_model(model_name)

    # ensure storage class
    caas_client.kubectl_apply(JUJU_STORAGECLASS_TEMPLATE.format(model=model_name, class_name=JUJU_STORAGECLASS_NAME))

    # ensure tmp dir for storage class.model_name
    o = subprocess.check_output(
        ('sudo', 'mkdir', '-p', '/mnt/data/%s' % model_name)  # unfortunately, needs sudo
    )
    log.debug(o.decode('UTF-8').strip())

    # ensure storage pool
    k8s_model.juju(
        'create-storage-pool',
        ('operator-storage', 'kubernetes', 'storage-class=%s' % JUJU_STORAGECLASS_NAME)
    )

    gitlab_charm_path = local_charm_path(charm='caas-gitlab', juju_ver=client.version)
    k8s_model.deploy(
        charm=gitlab_charm_path, config='juju-external-hostname={}'.format(external_hostname)
    )

    mysql_charm_path = local_charm_path(charm='caas-mysql', juju_ver=client.version)
    k8s_model.deploy(charm=mysql_charm_path)

    k8s_model.juju('relate', ('gitlab', 'mysql'))
    k8s_model.juju('expose', ('gitlab',))
    k8s_model.wait_for_workloads(timeout=3600)

    url = '{}://{}/{}'.format('http', external_hostname, 'gitlab')
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
