#!/usr/bin/env python
"""Test juju update-mongo command."""

from __future__ import print_function

__metaclass__ = type

import argparse
import logging
import sys

from deploy_stack import (
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


log = logging.getLogger("assess_update_mongo")


def assess_update_mongo(client, series):
    charm = 'local:{}/ubuntu'.format(series)
    log.info("Setting up test.")
    client.deploy(charm)
    client.wait_for_started()
    log.info("Setup complete.")


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test juju update-mongo command")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_update_mongo(bs_manager.client, args.series)
    return 0


if __name__ == '__main__':
    sys.exit(main())
