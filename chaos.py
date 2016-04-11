#!/usr/bin/env python
from collections import defaultdict
from contextlib import contextmanager
from datetime import (
    datetime,
    timedelta,
)
import logging
import os
import subprocess
import sys

from remote import remote_from_unit
from utility import (
    local_charm_path,
    until_timeout,
)


__metaclass__ = type


@contextmanager
def background_chaos(env, client, log_dir, time):
    monkey = MonkeyRunner(env, client, enablement_timeout=time)
    monkey.deploy_chaos_monkey()
    monkey_ids = monkey.unleash_once()
    monkey.wait_for_chaos(state='start')
    try:
        yield
        monkey.wait_for_chaos(state='complete', timeout=time)
    except BaseException as e:
        logging.exception(e)
        sys.exit(1)
    finally:
        # Copy the chaos logs to the log directory.
        # Get the remote machine. Currently the remote machine will always be
        # ubuntu/0. IF background_chaos() is enhanced to take a target service,
        # then log collection will also need to be updated.
        remote = remote_from_unit(client, "ubuntu/0")
        for id in monkey_ids:
            monkey_log = ['chaos-monkey/chaos_monkey.{}/log/*'.format(id)]
            dest_dir = '{}/chaos-monkey-{}'.format(log_dir, id)
            os.mkdir(dest_dir)
            try:
                remote.copy(dest_dir, monkey_log)
            except subprocess.CalledProcessError as e:
                logging.warning(
                    'Could not retrieve Chaos Monkey log for {}:'.format(id))
                logging.warning(e.output)


class MonkeyRunner:

    def __init__(self, env, client, service='0', health_checker=None,
                 enablement_timeout=120, pause_timeout=0, total_timeout=0):
        self.env = env
        if service == '0':
            self.service = 'ubuntu'
            self.machine = '0'
        else:
            self.service = service
            self.machine = None
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
        if self.machine:
            logging.debug(
                'Deploying ubuntu to machine {}.'.format(self.machine))
            charm = local_charm_path(
                charm='ubuntu', juju_ver=self.client.version)
            self.client.deploy(charm, to=self.machine)
        logging.debug('Deploying local:chaos-monkey.')
        charm = local_charm_path(
            charm='chaos-monkey', juju_ver=self.client.version)
        self.client.deploy(charm)
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

            id = self.client.action_do(*args)
            if not self.monkey_ids.get(unit_name):
                logging.info('Setting the monkey-id for {} to: {}'.format(
                    unit_name, id))
                self.monkey_ids[unit_name] = id
        return self.monkey_ids.values()

    def is_healthy(self):
        """Returns a boolean after running the health_checker."""
        if self.health_checker:
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

    def wait_for_chaos(self, state='complete', timeout=300):
        if not ('complete' in state or 'start' in state):
            raise Exception('Unexpected state value: {}'.format(state))
        for ignored in until_timeout(timeout):
            locks = defaultdict(list)
            for unit_name, unit in self.iter_chaos_monkey_units():
                locks[self.get_unit_status(unit_name)].append(unit_name)
            if state == 'complete' and locks.keys() == ['done']:
                logging.debug(
                    'All lock files removed, chaos complete: {}'.format(locks))
                break
            if state == 'start' and locks.keys() == ['running']:
                logging.debug(
                    'All lock files found, chaos started: {}'.format(locks))
                break
        else:
            raise Exception('Chaos operations did not {}.'.format(state))
