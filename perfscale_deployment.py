#!/usr/bin/env python
"""Perfscale test for general deployment measurements.

Steps taken in this test:
  - Bootstraps a single environment.
  - Deploys Bundle (defaults to: landscape-scalable).
  - Ensures workloads are up.
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
from utility import (
    configure_logging,
)

__metaclass__ = type


def assess_deployment_perf(client, args):
    """Deploy supplied bundle wait for it to come up."""
    deploy_start = datetime.utcnow()

    # We possibly want 2 timing details here, one for started (i.e. agents
    # ready) and the other for the workloads to be complete.
    client.deploy(args.bundle_name)
    client.wait_for_started()
    client.wait_for_workloads()

    deploy_end = datetime.utcnow()
    deploy_timing = TimingData(deploy_start, deploy_end)

    client_details = get_client_details(client)

    return DeployDetails(args.bundle_name, client_details, deploy_timing)


def get_client_details(client):
    """Get unit count details for all units.

    :return: Dict containing a name -> unit_count mapping.
    """
    status = client.get_status()
    units = dict()
    for name in status.get_applications().keys():
        units[name] = status.get_service_unit_count(name)
    return units


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Perfscale bundle deployment test.")
    add_basic_perfscale_arguments(parser)
    parser.add_argument(
        '--bundle-name',
        help='Bundle to deploy during test run.',
        default='cs:~landscape/bundle/landscape-scalable')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    run_perfscale_test(assess_deployment_perf, bs_manager, args)

    return 0

if __name__ == '__main__':
    sys.exit(main())
