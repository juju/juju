#!/usr/bin/env python
"""Assess mixed deployment of images two sets of simplestreams."""

from __future__ import print_function

import argparse
import logging
import os
import sys

from deploy_stack import (
    assess_juju_relations,
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


__metaclass__ = type


log = logging.getLogger("assess_mixed_images")

IMG_URL = 'https://s3.amazonaws.com/temp-streams/aws-image-streams/'


def assess_mixed_images(client):
    client.deploy('local:centos7/dummy-sink')
    client.deploy('local:trusty/dummy-source')
    client.juju('add-relation', ('dummy-source', 'dummy-sink'))
    # Wait for the deployment to finish.
    client.wait_for_started()
    assess_juju_relations(client)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="TODO: script info")
    add_basic_testing_arguments(parser)
    # Fallback behaviour fails without --bootstrap-series: Bug 1560625
    parser.set_defaults(series='trusty')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    client.env.config['image-metadata-url'] = IMG_URL
    key_path = os.path.join(client.env.juju_home,
                            'juju-qa-public.key')
    os.environ['JUJU_STREAMS_PUBLICKEY_FILE'] = key_path
    with bs_manager.booted_context(args.upload_tools):
        assess_mixed_images(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
