#!/usr/bin/env python3
""" Test webscale deployment

    1. deploying kubenetes core and asserting it is `healthy`
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
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    get_current_model,
)

from jujucharm import (
    local_charm_path,
)
from jujupy.utility import until_timeout

__metaclass__ = type

log = logging.getLogger("assess_deploy_webscale")

def deploy_bundle(client, charm_bundle):
    """Deploy the given charm bundle

    :param client: Jujupy ModelClient object
    """
    model_name = "webscale"
    current_model = client.add_model(model_name)
    current_model.deploy(
        charm=charm_bundle,
    )
    current_model.juju(current_model._show_status, ('--format', 'tabular'))
    current_model.wait_for_workloads(timeout=3600)
    current_model.juju(current_model._show_status, ('--format', 'tabular'))

    current_model.destroy_model()

def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Webscale charm deployment CI test")
    parser.add_argument(
        '--charm-bundle',
        help="Override the charm bundle to deploy",
    )
    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)

def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        client = bs_manager.client
        deploy_bundle(client, charm_bundle=args.charm_bundle)
    return 0

if __name__ == '__main__':
    sys.exit(main())
