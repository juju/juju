#!/usr/bin/env python
__metaclass__ = type

from argparse import ArgumentParser
import logging
import subprocess

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
)
from utility import configure_logging


class MonkeyRunner:

    @classmethod
    def from_config(cls, args):
        """Return a class instance populated with values from args.

        Lets EnvJujuClient.by_version() default to the juju binary in
        the OS path.
        """
        client = EnvJujuClient.by_version(
            SimpleEnvironment.from_config(args.env))
        return cls(args.env, args.service, args.health_checker, client)

    def __init__(self, env, service, health_checker, client):
        self.env = env
        self.service = service
        self.health_checker = health_checker
        self.client = client
        self.monkey_ids = []

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

    def unleash_once(self):
        status = self.client.get_status()
        for unit_name, unit in status.service_subordinate_units(self.service):
            if not unit_name.startswith('chaos-monkey'):
                continue
            logging.info('Starting the chaos monkey on: {}'.format(unit_name))
            action_out = self.client.get_juju_output(
                'action do', unit_name, 'start', 'mode=single')
            if not action_out.startswith('Action queued with id'):
                raise Exception(
                    'Unexpected output from "juju action do": {}'.format(
                        action_out))
            logging.info(action_out)
            self.monkey_ids.append(action_out.split().pop())

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


def get_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('env', help='The name of the environment.')
    parser.add_argument('service', help='A service name to monkey with.')
    parser.add_argument(
        'health_checker',
        help='A binary for checking the health of the environment.')
    args = parser.parse_args(argv)
    return args


def main():
    """ Deploy and run chaos monkey, while checking env health.

    The Chaos Monkey is deployed into the environment and related to
    the specified service. Juju actions are then used to run one choas
    operation at a time. Once the operchaos monkey"""
    configure_logging(logging.DEBUG)
    args = get_args()
    monkey_runner = MonkeyRunner.from_config(args)
    monkey_runner.deploy_chaos_monkey()
    monkey_runner.unleash_once()

if __name__ == '__main__':
    main()
