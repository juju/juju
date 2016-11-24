#!/usr/bin/env python
"""TODO: add rough description of what is assessed in this module."""

from __future__ import print_function

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


__metaclass__ = type


log = logging.getLogger("assess_model_change_watcher")


def assess_model_change_watcher(client):
    # Deploy charms, there are several under ./repository
    client.deploy('dummy-source')
    # Wait for the deployment to finish.
    client.wait_for_started()
    token = "hello"
    client.set_config('dummy-source', {'token': token})


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Model change watcher")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_model_change_watcher(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
