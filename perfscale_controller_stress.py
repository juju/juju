#!/usr/bin/env python
"""Perfscale test that uses the observable-swarm charm collection to exercise
the controller(s).

Steps taken in this test:
  - Bootstraps a new controller (installing monitoring software on it).
  - Iterate a user defined amount:
    - Add new model
    - Deploy observable-swarm to this new model
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


def assess_controller_stress(client, args):
    """Deploy observable-swarm charm many times to stress test controller
    machines.
    """
    test_start = datetime.utcnow()

    deploy_details = dict()
    for model_number in xrange(0, args.deploy_amount):
        model_name = 'swarm-model-{}'.format(model_number)
        deploy_time = deploy_swarm_to_new_model(client, model_name)
        deploy_details[model_name] = '{} Seconds'.format(deploy_time)

    test_end = datetime.utcnow()
    deploy_timing = TimingData(test_start, test_end)

    return DeployDetails('Controller Stress.', deploy_details, deploy_timing)


def deploy_swarm_to_new_model(client, model_name):
    before_add = datetime.utcnow()

    new_client = client.add_model(model_name)
    new_client.deploy(get_charm_url())
    new_client.wait_for_started()
    new_client.wait_for_workloads()

    after_add = datetime.utcnow()
    return int((after_add - before_add).total_seconds())


def get_charm_url():
    return 'cs:bundle/observable-swarm-1'


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Perfscale Controller Stress test.')
    add_basic_perfscale_arguments(parser)
    parser.add_argument(
        '--deploy-amount',
        help='The amount of deploys to do.',
        type=int,
        default=1)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    run_perfscale_test(assess_controller_stress, bs_manager, args)

    return 0

if __name__ == '__main__':
    sys.exit(main())
