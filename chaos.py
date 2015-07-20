#!/usr/bin/env python
__metaclass__ = type


from collections import defaultdict
from datetime import (
    datetime,
    timedelta,
)
import logging
from time import sleep
import subprocess
import sys

from utility import (
    until_timeout,
)


class MonkeyRunner:

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
            monkey_id = self.monkey_ids.get(unit_name)
            args = (unit_name,) + ('start',) + ('mode=single',)
            args = args + (enablement_arg,)
            if monkey_id is not None:
                args = args + ('monkey-id={}'.format(monkey_id),)
            action_out = self.client.get_juju_output('action do', *args)
            if not action_out.startswith('Action queued with id'):
                raise Exception(
                    'Unexpected output from "juju action do": {}'.format(
                        action_out))
            logging.info(action_out)
            if not self.monkey_ids.get(unit_name):
                id = action_out.split().pop()
                logging.info('Setting the monkey-id for {} to: {}'.format(
                    unit_name, id))
                self.monkey_ids[unit_name] = id
        # Allow chaos time to run
        sleep(self.enablement_timeout)

    def is_healthy(self):
        """Returns a boolean after running the health_checker."""
        try:
            sub_output = subprocess.check_output(self.health_checker)
            logging.info('Health check output: {}'.format(sub_output))
        except OSError as e:
            logging.error(
                'The health check failed to execute with: {}'.format(
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
