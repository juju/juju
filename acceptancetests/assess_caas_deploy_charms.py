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
from time import sleep

import requests

from deploy_stack import BootstrapManager
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
)

from jujupy.utility import until_timeout
from jujupy.k8s_provider import (
    providers,
    K8sProviderType,
)

__metaclass__ = type


log = logging.getLogger("assess_caas_charm_deployment")


def check_app_healthy(url, timeout=300, success_hook=lambda: None, fail_hook=lambda: None):
    if not callable(success_hook) or not callable(fail_hook):
        raise RuntimeError("hooks are not callable")

    status_code = None
    for remaining in until_timeout(timeout):
        try:
            r = requests.get(url)
            if r.ok and r.status_code < 400:
                return success_hook()
            status_code = r.status_code
        except IOError as e:
            log.error(e)
        finally:
            sleep(3)
            if remaining % 30 == 0:
                log.info('timeout in %ss', remaining)
    log.error('HTTP health check failed -> %s, status_code -> %s !', url, status_code)
    fail_hook()
    raise JujuAssertionError('gitlab is not healthy')


def assess_caas_charm_deployment(caas_client):
    external_hostname = caas_client.get_external_hostname()

    if not caas_client.check_cluster_healthy(timeout=60):
        raise JujuAssertionError('k8s cluster is not healthy because kubectl is not accessible')

    # add caas model for deploying caas charms on top of it
    model_name = 'testcaas'
    k8s_model = caas_client.add_model(model_name)

    sc_name = caas_client.default_storage_class_name
    # ensure storage pools for caas operator using default sc.
    k8s_model.juju(
        'create-storage-pool',
        ('operator-storage', 'kubernetes', 'storage-class=%s' % sc_name)
    )

    # ensure storage pools for mariadb
    # TODO(ycliuhw): remove storage-pool setup, because Juju takes care of it now.
    # And add tests for deploy with & without storage setup.
    mariadb_storage_pool_name = 'mariadb-pv'
    k8s_model.juju(
        'create-storage-pool',
        (mariadb_storage_pool_name, 'kubernetes', 'storage-class=%s' % sc_name)
    )

    k8s_model.deploy(
        charm="cs:~juju/gitlab-k8s-0",
        config='juju-external-hostname={}'.format(external_hostname),
    )

    k8s_model.deploy(
        charm="cs:~juju/mariadb-k8s-0",
        storage='database=100M,{}'.format(mariadb_storage_pool_name),
    )

    k8s_model.juju('relate', ('mariadb-k8s', 'gitlab-k8s'))
    k8s_model.juju('expose', ('gitlab-k8s',))
    k8s_model.wait_for_workloads(timeout=3600)

    def success_hook():
        log.info(caas_client.kubectl('get', 'all', '--all-namespaces'))

    def fail_hook():
        success_hook()
        log.info(caas_client.kubectl('get', 'pv,pvc', '-n', model_name))

    url = '{}://{}/{}'.format('http', external_hostname, 'gitlab-k8s')
    check_app_healthy(
        url, timeout=1800,
        success_hook=success_hook,
        fail_hook=fail_hook,
    )
    k8s_model.juju(k8s_model._show_status, ('--format', 'tabular'))


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="CAAS charm deployment CI test")
    parser.add_argument(
        '--caas-image-repo', action='store', default='jujuqabot',
        help='CAAS operator docker image repo to use.'
    )
    parser.add_argument(
        '--caas-provider', action='store', default='MICROK8S',
        choices=K8sProviderType.keys(),
        help='Specify K8s cloud provider to use for CAAS tests.'
    )

    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)


def ensure_operator_image_path(client, caas_image_repo):
    client.controller_juju('controller-config', ('caas-image-repo={}'.format(caas_image_repo),))


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        client = bs_manager.client
        ensure_operator_image_path(client, caas_image_repo=args.caas_image_repo)
        k8s_provider = providers[args.caas_provider]
        caas_client = k8s_provider(client)
        assess_caas_charm_deployment(caas_client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
