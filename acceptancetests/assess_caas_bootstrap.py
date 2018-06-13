#!/usr/bin/env python3
""" Test caas k8s cluster bootstrap

    Spinning up k8s cluster and asserting the cluster is `healthy` - in this case, healthy means the k8s api server
    is functionally working well, further health check needs to be done by charm deployment in next step
"""

from __future__ import print_function

import argparse
import logging
import sys
import os

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

__metaclass__ = type


log = logging.getLogger("assess_caas_bootstrap")


def assess_caas_bootstrap(client):
    # Deploy k8s bundle to spin up k8s cluster
    bundle = local_charm_path(
        charm='bundles-kubernetes-core-lxd.yaml',
        repository=os.environ['JUJU_REPOSITORY'],
        juju_ver=client.version
    )

    caas_client = deploy_caas_stack(bundle_path=bundle, client=client, timeout=3600)
    if not caas_client.is_cluster_healthy:
        raise JujuAssertionError('k8s cluster is not healthy coz kubectl is not accessible')


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Cass Bootstrap CI test")

    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_caas_bootstrap(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
