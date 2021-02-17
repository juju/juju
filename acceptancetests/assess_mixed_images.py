#!/usr/bin/env python3
"""Assess mixed deployment of images from two sets of simplestreams."""

from __future__ import print_function

import argparse
import logging
import sys

from deploy_stack import (
    assess_juju_relations,
    BootstrapManager,
)
from jujucharm import (
    local_charm_path,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


__metaclass__ = type


log = logging.getLogger("assess_mixed_images")


def assess_mixed_images(client):
    charm_path = local_charm_path(charm='dummy-sink', juju_ver=client.version,
                                  series='centos7', platform='centos')
    client.deploy(charm_path)
    charm_path = local_charm_path(charm='dummy-source',
                                  juju_ver=client.version, series='trusty')
    client.deploy(charm_path)
    client.juju('add-relation', ('dummy-source', 'dummy-sink'))
    # Wait for the deployment to finish.
    client.wait_for_started()
    assess_juju_relations(client)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Deploy images from two sets of simplestreams.")
    add_basic_testing_arguments(parser)
    # Fallback behaviour fails without --bootstrap-series: Bug 1560625
    parser.set_defaults(series='trusty')
    parser.add_argument('--image-metadata-url')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    if args.image_metadata_url is not None:
        client.env.update_config('image-metadata-url',
                                 args.image_metadata_url)
    with bs_manager.booted_context(args.upload_tools):
        assess_mixed_images(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
