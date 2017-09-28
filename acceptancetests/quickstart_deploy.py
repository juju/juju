#!/usr/bin/env python
from argparse import ArgumentParser
import logging
import sys

from deploy_stack import (
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    LoggedException,
)


__metaclass__ = type


class QuickstartTest:

    def __init__(self, bs_manager, bundle_path, service_count):
        self.bs_manager = bs_manager
        self.client = bs_manager.client
        self.bundle_path = bundle_path
        self.service_count = service_count

    def run(self):
        bootstrap_host = None
        try:
            step_iter = self.iter_steps()
            for step in step_iter:
                try:
                    logging.info('{}'.format(step))
                    if not bootstrap_host:
                        bootstrap_host = step.get('bootstrap_host')
                except BaseException as e:
                    step_iter.throw(e)
        finally:
            step_iter.close()

    def iter_steps(self):
        # Start the quickstart job
        step = {'juju-quickstart': 'Returned from quickstart'}
        with self.bs_manager.top_context() as machines:
            with self.bs_manager.bootstrap_context(machines):
                self.client.quickstart(self.bundle_path)
                yield step
            with self.bs_manager.runtime_context(machines):
                # Get the hostname for machine 0
                step['bootstrap_host'] = self.bs_manager.known_hosts['0']
                yield step
                # Wait for deploy to start
                self.client.wait_for_deploy_started(self.service_count)
                step['deploy_started'] = 'Deploy stated'
                yield step
                # Wait for all agents to start
                self.client.wait_for_started(3600)
                step['agents_started'] = 'All Agents started'
                yield step


def main():
    parser = add_basic_testing_arguments(ArgumentParser())
    parser.add_argument('bundle_path',
                        help='URL or path to a bundle')
    parser.add_argument('--service-count', type=int, default=2,
                        help='Minimum number of expected services.')
    args = parser.parse_args()
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    quickstart = QuickstartTest(
        bs_manager, args.bundle_path, args.service_count)
    try:
        quickstart.run()
    except LoggedException:
        sys.exit(1)
    except Exception as e:
        print('%s (%s)' % (e, type(e).__name__))
        sys.exit(1)


if __name__ == '__main__':
    main()
