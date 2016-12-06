#!/usr/bin/env python
"""Perfscale test that uses the xplod charm to exercise the controllers.

Steps taken in this test:
  - Bootstraps a single environment.
  - Iterate for current max of xplod deploys (i.e. 7)
    - Add a new model
      - On that model deploy xplod charm
"""

import argparse
from datetime import datetime
import sys

from deploy_stack import (
    BootstrapManager,
)
from generate_perfscale_results import (
    DeployDetails,
    TimingData,
    add_basic_perfscale_arguments,
    run_perfscale_test,
)
from jujucharm import local_charm_path
from utility import (
    configure_logging,
)

__metaclass__ = type


def assess_xplod_perf(client, args):
    """Deploy xplod charm many times."""
    deploy_start = datetime.utcnow()

    deploy_xplod_charm(client)
    client_details = add_multiple_units(client, args)

    deploy_end = datetime.utcnow()
    deploy_timing = TimingData(deploy_start, deploy_end)

    return DeployDetails('Xplod charm', client_details, deploy_timing)


def deploy_xplod_charm(client):
    charm_path = local_charm_path(charm='peer-xplod', juju_ver=client.version)

    client.deploy(charm_path, series='trusty')
    client.wait_for_started()
    client.wait_for_workloads()


def add_multiple_units(client, args):
    unit_deploys = dict()
    total_units = 0
    # We will want to add units in different ways (e.g. 2 at a time). Keep this
    # open for extension.
    for additional_amount in singular_unit(args.deploy_amount):
        before_add = datetime.utcnow()

        client.juju('add-unit', ('peer-xplod', '-n', str(additional_amount)))
        client.wait_for_started()
        client.wait_for_workloads(timeout=1200)

        after_add = datetime.utcnow()
        total_seconds = int((after_add - before_add).total_seconds())

        total_units += additional_amount
        unit_deploys['unit-{}'.format(total_units)] = '{} Seconds'.format(
            total_seconds)
    return unit_deploys


def singular_unit(count):
    """Add a single unit each time until added `count` amount.

    Yield 1 `count` amount of times.
    """
    for _ in xrange(0, count):
        yield 1


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Perfscale xplod charm stree test.')
    add_basic_perfscale_arguments(parser)
    parser.add_argument(
        '--deploy-amount',
        help='The amount of deploys of xplod charm to do.',
        type=int,
        default=7)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    run_perfscale_test(assess_xplod_perf, bs_manager, args)

    return 0

if __name__ == '__main__':
    sys.exit(main())
