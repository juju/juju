#!/usr/bin/env python3
""" Test caas k8s cluster bootstrap

    1. spinning up k8s cluster and asserting the cluster is `healthy`;
    2. deploy gitlab, mysql charms to caas model;
    3. relate gitlab mysql;
    4. assert http health check on gitlab
"""

from __future__ import print_function

import argparse
import json
import logging
import sys
from pprint import pformat
from time import sleep

import requests
from deploy_stack import BootstrapManager
from utility import (
    JujuAssertionError, add_basic_testing_arguments, configure_logging,
)

from jujupy.k8s_provider import K8sProviderType, providers
from jujupy.utility import until_timeout

__metaclass__ = type


log = logging.getLogger("assess_caas_charm_deployment")


def check_app_healthy(url, timeout=300, success_hook=lambda: None,
                      fail_hook=lambda: None):
    if not callable(success_hook) or not callable(fail_hook):
        raise RuntimeError("hooks are not callable")

    status_code = None
    for remaining in until_timeout(timeout):
        try:
            r = requests.get(url)
            status_code = r.status_code
            if r.ok and status_code < 400:
                return success_hook()
        except IOError:
            ...
        finally:
            sleep(3)
            if remaining % 30 == 0:
                log.info('timeout in %ss', remaining)
    log.error('HTTP health check failed -> %s, status_code -> %s !', url,
              status_code)
    fail_hook()
    raise JujuAssertionError('%s is not healthy' % url)


def get_app_endpoint(caas_client, model_name, app_name, svc_type, timeout=180):
    if svc_type == 'LoadBalancer':
        for remaining in until_timeout(timeout):
            try:
                lb_addr = caas_client.get_lb_svc_address(app_name, model_name)
                if lb_addr:
                    log.info('load balancer addr for {} is {}'.format(
                        app_name, lb_addr))
                    return lb_addr
            except:  # noqa: E722
                continue
        raise JujuAssertionError(
            'No load balancer addr available for {}'.format(app_name))

    return caas_client.get_external_hostname()


def deploy_test_workloads(caas_client, k8s_model, caas_provider):
    k8s_model.deploy(charm="cs:~juju/mariadb-k8s-3")
    svc_type = None
    if caas_provider == K8sProviderType.MICROK8S.name:
        k8s_model.deploy(
            charm="cs:~juju/mediawiki-k8s-4",
            config='juju-external-hostname={}'.format(
                caas_client.get_external_hostname()),
        )
        k8s_model.juju('expose', ('mediawiki-k8s',))
    else:
        k8s_model.deploy(
            charm="cs:~juju/mediawiki-k8s-4",
            config='kubernetes-service-type=loadbalancer',
        )
        svc_type = 'LoadBalancer'

    k8s_model.juju('relate', ('mediawiki-k8s:db', 'mariadb-k8s:server'))
    k8s_model.wait_for_workloads(timeout=600)
    return 'http://' + get_app_endpoint(caas_client, k8s_model.model_name,
                                        'mediawiki-k8s', svc_type)


def assess_caas_charm_deployment(caas_client, caas_provider):
    if not caas_client.check_cluster_healthy(timeout=60):
        raise JujuAssertionError(("k8s cluster is not healthy because kubectl "
                                  "is not accessible"))

    model_name = caas_client.client.env.controller.name + '-test-caas-model'
    k8s_model = caas_client.add_model(model_name)

    def success_hook():
        log.info(caas_client.kubectl('get', 'all,pv,pvc,ing', '--all-namespaces', '-o', 'wide'))

    def fail_hook():
        success_hook()
        ns_dumps = caas_client.kubectl('get', 'all,pv,pvc,ing', '-n', model_name, '-o', 'json')
        log.info('all resources in namespace %s -> %s', model_name, pformat(json.loads(ns_dumps)))
        log.info(caas_client.kubectl('get', 'pv,pvc', '-n', model_name))
        caas_client.ensure_cleanup()

    try:
        endpoint = deploy_test_workloads(caas_client, k8s_model, caas_provider)
        log.info("sleeping for 30 seconds to let everything start up")
        sleep(30)
        check_app_healthy(
            endpoint, timeout=600,
            success_hook=success_hook,
            fail_hook=fail_hook,
        )
        k8s_model.juju(k8s_model._show_status, ('--format', 'tabular'))
    except:  # noqa: E722
        # run cleanup steps then raise.
        fail_hook()
        raise


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="CAAS charm deployment CI test")
    parser.add_argument(
        '--caas-image-repo', action='store', default='jujuqabot',
        help='CAAS operator docker image repo to use.'
    )
    parser.add_argument(
        '--caas-provider', action='store', default='MICROK8S',
        choices=K8sProviderType.keys(),
        help='Specify K8s cloud provider to use for CAAS tests.'
    )
    parser.add_argument(
        '--k8s-controller',
        action='store_true',
        help='Bootstrap to k8s cluster or not.'
    )

    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    k8s_provider = providers[args.caas_provider]
    bs_manager = BootstrapManager.from_args(args)

    with k8s_provider(bs_manager, cluster_name=args.temp_env_name).substrate_context() as caas_client:
        # add-k8s --local
        is_mk8s = args.caas_provider == K8sProviderType.MICROK8S.name
        if args.k8s_controller and not is_mk8s:
            # microk8s is built-in cloud, no need run add-k8s for bootstrapping
            caas_client.add_k8s(True)
        with bs_manager.existing_booted_context(args.upload_tools,
             caas_image_repo=args.caas_image_repo):
            if not args.k8s_controller:
                # add-k8s to controller
                caas_client.add_k8s(False)
            assess_caas_charm_deployment(caas_client, args.caas_provider)
        return 0


if __name__ == '__main__':
    sys.exit(main())
