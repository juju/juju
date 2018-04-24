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
from jujupy import ClientType

__metaclass__ = type


log = logging.getLogger("assess_caas_bootstrap")


def assess_caas_bootstrap(client):
    # Deploy k8s bundle to spin up k8s cluster
    bundle = local_charm_path(charm='bundles-kubernetes-core-lxd.yaml',
                              repository=os.environ['JUJU_REPOSITORY'], juju_ver=client.version)
    client.cluster_up(bundle)

    k8s_model = client.add_caas_model('testcaas')  # noqa
    client.check_healthy()

    # TODO: deploy charms


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser()
    parser.add_argument('--client_type', default=ClientType.Caas)

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
