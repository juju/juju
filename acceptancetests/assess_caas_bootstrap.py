#!/usr/bin/env python
""" Test caas k8s cluster bootstrap
"""

from __future__ import print_function

import argparse
import logging
import sys
import os

from deploy_stack import (
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)

from jujucharm import (
    local_charm_path
)


__metaclass__ = type


log = logging.getLogger("assess_caas_bootstrap")


def assess_caas_bootstrap(client):
    # Deploy k8s bundle to spin up k8s cluster
    repository_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'repository')
    bundle = local_charm_path(charm='bundles-kubernetes-core-lxd.yaml',
                              repository=repository_dir, juju_ver=client.version)
    client.deploy_bundle(bundle, static_bundle=True)
    # Wait for the deployment to finish.
    client.wait_for_started()

    # ensure kube credentials
    client.scp(_from='kubernetes-master/0:config', _to='~/.kube/config')

    # TODO:
    # 0. assert cluster healthy
    # 1. add cloud
    # 2. add k8s model
    # 3. deploy charms


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser()
    # TODO: Add additional positional arguments.
    # NOTE: If this test does *not* support running on an existing bootstrapped
    #   controller, pass `existing=False` to add_basic_testing_arguments.
    add_basic_testing_arguments(parser, existing=False)
    # TODO: Add additional optional arguments.
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
