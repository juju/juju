#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
import logging
import sys

from deploy_stack import (
    BootstrapManager,
    tear_down,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    )


log = logging.getLogger("assess_bootstrap")


def assess_bootstrap(args):
    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    with bs_manager.top_context() as machines:
        with bs_manager.bootstrap_context(machines):
            tear_down(client, client.is_jes_enabled())
            client.bootstrap()
        with bs_manager.runtime_context(machines):
            client.get_status(1)
            log.info('Environment successfully bootstrapped.')


def parse_args(argv=None):
    parser = ArgumentParser(description='Test the bootstrap command.')
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    assess_bootstrap(args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
