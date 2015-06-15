#!/usr/bin/env python
__metaclass__ = type

from argparse import ArgumentParser
from collections import defaultdict
from datetime import (
    datetime,
    timedelta,
)
import logging
import subprocess
import sys
from time import sleep

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
)
from utility import (
    configure_logging,
    until_timeout,
)


class MonkeyRunner:

    @classmethod
    def from_config(cls, args):
        """Return a class instance populated with values from args.

        Lets EnvJujuClient.by_version() default to the juju binary in
        the OS path.
        """
        client = EnvJujuClient.by_version(
            SimpleEnvironment.from_config(args.env))
        return cls(args.env, args.service, args.health_checker, client,
                   enablement_timeout=args.enablement_timeout,
                   pause_timeout=args.pause_timeout,
                   total_timeout=args.total_timeout)

    def __init__(self, env, service, health_checker, client,
                 enablement_timeout=0, pause_timeout=0, total_timeout=0):
        self.env = env
        self.service = service
        self.health_checker = health_checker
        self.client = client
        self.enablement_timeout = enablement_timeout
        self.pause_timeout = pause_timeout
        self.total_timeout = total_timeout
        self.expire_time = (datetime.now() + timedelta(seconds=total_timeout))
        self.monkey_ids = {}

    def deploy_chaos_monkey(self):
        """Juju deploy chaos-monkey and add a relation.

        JUJU_REPOSITORY must be set in the OS environment so a local
        chaos-monkey charm can be found.
        """
        logging.debug('Deploying local:chaos-monkey.')
        self.client.deploy('local:chaos-monkey')
        logging.debug('Relating chaos-monkey to {}.'.format(self.service))
        self.client.juju('add-relation', (self.service, 'chaos-monkey'))
        logging.debug('Waiting for services to start.')
        self.client.wait_for_started()
        self.client.wait_for_subordinate_units(self.service, 'chaos-monkey')

    def iter_chaos_monkey_units(self):
        status = self.client.get_status()
        for unit_name, unit in status.service_subordinate_units(self.service):
            if not unit_name.startswith('chaos-monkey'):
                continue
            yield unit_name, unit

    def unleash_once(self):
        for unit_name, unit in self.iter_chaos_monkey_units():
            logging.info('Starting the chaos monkey on: {}'.format(unit_name))
            enablement_arg = ('enablement-timeout={}'.format(
                self.enablement_timeout))
            action_out = self.client.get_juju_output(
                'action do', unit_name, 'start', 'mode=single', enablement_arg)
            if not action_out.startswith('Action queued with id'):
                raise Exception(
                    'Unexpected output from "juju action do": {}'.format(
                        action_out))
            logging.info(action_out)
            self.monkey_ids[unit_name] = action_out.split().pop()
        # Allow chaos time to run
        sleep(self.enablement_timeout)

    def is_healthy(self):
        """Returns a boolean after running the health_checker."""
        try:
            sub_output = subprocess.check_output(self.health_checker)
            logging.info(sub_output)
        except OSError as e:
            logging.error(
                'The health check script failed to execute with: {}'.format(
                    e))
            raise
        except subprocess.CalledProcessError as e:
            logging.error('Non-zero exit code returned from {}: {}'.format(
                self.health_checker, e))
            logging.error(e.output)
            return False
        return True

    def get_unit_status(self, unit_name):
        """Return 'done' if no lock file otherwise 'running'"""
        service_config = self.client.get_service_config('chaos-monkey')
        logging.debug('{}'.format(service_config))
        logging.debug('Checking if chaos is done on: {}'.format(unit_name))
        check_cmd = '[ -f '
        check_cmd += service_config['settings']['chaos-dir']['value']
        check_cmd += '/chaos_monkey.' + self.monkey_ids[unit_name]
        check_cmd += '/chaos_runner.lock'
        check_cmd += ' ]'
        if self.client.juju('run', ('--unit', unit_name, check_cmd),
                            check=False):
            return 'done'
        return 'running'

    def wait_for_chaos_complete(self, timeout=300):
        for ignored in until_timeout(timeout):
            locks = defaultdict(list)
            for unit_name, unit in self.iter_chaos_monkey_units():
                locks[self.get_unit_status(unit_name)].append(unit_name)
            if locks.keys() == ['done']:
                logging.debug(
                    'All lock files have been removed: {}'.format(locks))
                break
        else:
            raise Exception('Chaos operations did not complete.')

    def run_while_healthy_or_timeout(self):
        logging.debug('run_while_healthy_or_timeout')
        while self.is_healthy():
            logging.debug('Unleashing chaos.')
            self.unleash_once()
            self.wait_for_chaos_complete()
            if datetime.now() > self.expire_time:
                logging.debug(
                    'Reached run timeout, all done running chaos.')
                break
            if self.pause_timeout:
                logging.debug(
                    'Pausing {} seconds after running chaos.'.format(
                        self.pause_timeout))
                sleep(self.pause_timeout)
        else:
            logging.error('The health check reported an error: {}'.format(
                self.health_checker))
            sys.exit(1)


def get_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('env', help='The name of the environment.')
    parser.add_argument('service', help='A service name to monkey with.')
    parser.add_argument(
        'health_checker',
        help='A binary for checking the health of the environment.')
    parser.add_argument(
        '-et', '--enablement-timeout', default=30, type=int,
        help="Enablement timeout in seconds.", metavar='SECONDS')
    parser.add_argument(
        '-tt', '--total-timeout', type=int, help="Total timeout in seconds.",
        metavar='SECONDS')
    parser.add_argument(
        '-pt', '--pause-timeout', default=0, type=int,
        help="Pause timeout in seconds.", metavar='SECONDS')
    args = parser.parse_args(argv)
    if not args.total_timeout:
        args.total_timeout = args.enablement_timeout
    if args.enablement_timeout > args.total_timeout:
        parser.error("total-timeout can not be less than "
                     "enablement-timeout.")
    if args.total_timeout <= 0:
        parser.error("Invalid total-timeout value: timeout must be "
                     "greater than zero.")
    if args.enablement_timeout < 0:
        parser.error("Invalid enablement-timeout value: timeout must be "
                     "zero or greater.")

    return args


def main():
    """ Deploy and run chaos monkey, while checking env health.

    The Chaos Monkey is deployed into the environment and related to
    the specified service. Juju actions are then used to run one chaos
    operation at a time. After each operation, the provided health
    check script is executed, to ensure the Juju environment or
    software stack is still healthy.
    """
    configure_logging(logging.INFO)
    args = get_args()
    monkey_runner = MonkeyRunner.from_config(args)
    logging.info("Chaos Monkey Start.")
    monkey_runner.deploy_chaos_monkey()
    monkey_runner.run_while_healthy_or_timeout()
    logging.info("Chaos Monkey Complete.")

if __name__ == '__main__':
    main()
