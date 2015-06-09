#!/usr/bin/env python
__metaclass__ = type

from argparse import ArgumentParser
from collections import defaultdict
from itertools import chain
import logging
import subprocess
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
                   enablement_timeout=args.enablement_timeout)

    def __init__(self, env, service, health_checker, client,
                 enablement_timeout=0):
        self.enablement_timeout = enablement_timeout
        self.env = env
        self.service = service
        self.health_checker = health_checker
        self.client = client
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
            enablement_arg = ('enablement_timeout=' +
                              str(self.enablement_timeout))
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

    def get_locks(self):
        """Return a dict with two lists: done and running"""
        locks = defaultdict(list)
        service_config = self.client.get_service_config(self.service)
        for unit_name, unit in self.iter_chaos_monkey_units():
            logging.debug('Checking if chaos is done on: {}'.format(unit_name))
            check_cmd = '[ -f '
            check_cmd += service_config['settings']['chaos-dir']['value']
            check_cmd += '/chaos_monkey.' + self.monkey_ids[unit_name]
            check_cmd += '/chaos_runner.lock'
            try:
                self.client.juju('run', ('--unit', unit_name, check_cmd))
                locks['running'].append(unit_name)
            except subprocess.CalledProcessError:
                locks['done'].append(unit_name)
        return locks

    def wait_for_chaos_complete(self):
        for ignored in chain([None], until_timeout(60)):
            for unit_name, unit in self.iter_chaos_monkey_units():
                logging.debug(
                    'Checking if chaos is done on: {}'.format(unit_name))
                locks = self.get_locks()
                if locks.keys() == ['done']:
                    return None
        else:
            raise Exception('Chaos operations did not complete.')


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
    args = parser.parse_args(argv)
    return args


def main():
    """ Deploy and run chaos monkey, while checking env health.

    The Chaos Monkey is deployed into the environment and related to
    the specified service. Juju actions are then used to run one choas
    operation at a time. Once the operchaos monkey"""
    configure_logging(logging.INFO)
    args = get_args()
    monkey_runner = MonkeyRunner.from_config(args)
    monkey_runner.deploy_chaos_monkey()
    monkey_runner.unleash_once()
    monkey_runner.wait_for_chaos_complete()

if __name__ == '__main__':
    main()
