#!/usr/bin/env python
__metaclass__ = type

from argparse import ArgumentParser
from collections import OrderedDict
from contextlib import contextmanager
from datetime import datetime
import json
import logging
import os
import sys
from textwrap import dedent

import yaml

from deploy_stack import (
    get_machine_dns_name,
    wait_for_state_server_to_shutdown,
    )
from jujuconfig import (
    get_juju_home,
    )
from jujupy import (
    AgentsNotStarted,
    EnvJujuClient,
    SimpleEnvironment,
    temp_bootstrap_env,
    uniquify_local,
    )
from substrate import (
    make_substrate_manager as real_make_substrate_manager,
    terminate_instances,
    )
from utility import (
    configure_logging,
    temp_dir,
    until_timeout,
    )


QUICK = 'quick'
DENSITY = 'density'
FULL = 'full'
BACKUP = 'backup'
UPGRADE = 'upgrade'


class StageInfo:

    def __init__(self, stage_id, title, report_on=True):
        self.stage_id = stage_id
        self.title = title
        self.report_on = report_on

    def as_tuple(self):
        return (self.stage_id, {
            'title': self.title, 'report_on': self.report_on})

    def as_result(self, result=None):
        result_dict = {'test_id': self.stage_id}
        if result is not None:
            result_dict['result'] = result
        return result_dict


class MultiIndustrialTest:
    """Run IndustrialTests until desired number of results are achieved.

    :ivar env: The name of the environment to use as a base.
    :ivar new_juju_path: Path to the non-system juju.
    :ivar stages: A list of StageAttempts.
    :ivar attempt_count: The number of attempts needed for each stage.
    """

    @classmethod
    def from_args(cls, args, suite):
        config = SimpleEnvironment.from_config(args.env).config
        stages = cls.get_stages(suite, config)
        return cls(args.env, args.new_juju_path,
                   stages, args.attempts, args.attempts * 2,
                   args.new_agent_url, args.debug, args.old_stable)

    @staticmethod
    def get_stages(suite, config):
        stages = list(suites[suite])
        return stages

    def __init__(self, env, new_juju_path, stages, attempt_count=2,
                 max_attempts=1, new_agent_url=None, debug=False,
                 really_old_path=None):
        self.env = env
        self.really_old_path = really_old_path
        self.new_juju_path = new_juju_path
        self.new_agent_url = new_agent_url
        self.stages = stages
        self.attempt_count = attempt_count
        self.max_attempts = max_attempts
        self.debug = debug

    def make_results(self):
        """Return a results list for use in run_tests."""
        results = []
        for stage in self.stages:
            for test_id, info in stage.get_test_info().items():
                results.append({
                    'title': info['title'],
                    'test_id': test_id,
                    'report_on': info.get('report_on', True),
                    'attempts': 0,
                    'old_failures': 0,
                    'new_failures': 0,
                    })
        return {'results': results}

    def run_tests(self):
        """Run all stages until required number of attempts are achieved.

        :return: a list of dicts describing output.
        """
        results = self.make_results()
        for unused_ in range(self.max_attempts):
            if results['results'][-1]['attempts'] >= self.attempt_count:
                break
            industrial = self.make_industrial_test()
            self.update_results(industrial.run_attempt(), results)
        return results

    @staticmethod
    def combine_results(result_list_list):
        combine_dict = OrderedDict()
        for result_list in result_list_list:
            for result in result_list['results']:
                test_id = result['test_id']
                if test_id not in combine_dict:
                    combine_dict[test_id] = result
                    continue
                existing_result = combine_dict[test_id]
                for key in ['attempts', 'old_failures', 'new_failures']:
                    existing_result[key] += result[key]
                existing_result['report_on'] = (
                    result.get('report_on', True) or
                    existing_result.get('report_on', True)
                    )
        return {'results': combine_dict.values()}

    def make_industrial_test(self):
        """Create an IndustrialTest for this MultiIndustrialTest."""
        stable_path = EnvJujuClient.get_full_path()
        paths = [self.really_old_path, stable_path, self.new_juju_path]
        upgrade_sequence = [p for p in paths if p is not None]
        stage_attempts = [stage.factory(upgrade_sequence)
                          for stage in self.stages]
        return IndustrialTest.from_args(self.env, self.new_juju_path,
                                        stage_attempts, self.new_agent_url,
                                        self.debug)

    def update_results(self, run_attempt, results):
        """Update results with data from run_attempt.

        Results for stages that have already reached self.attempts are
        ignored.
        """
        for result, cur_result in zip(results['results'], run_attempt):
            if result['attempts'] >= self.attempt_count:
                continue
            if result['test_id'] != cur_result[0]:
                raise Exception('Mismatched result ids: {} != {}'.format(
                    cur_result[0], result['test_id']))
            result['attempts'] += 1
            if not cur_result[1]:
                result['old_failures'] += 1
            if not cur_result[2]:
                result['new_failures'] += 1

    @staticmethod
    def results_table(results):
        """Yield strings for a human-readable table of results."""
        yield 'old failure | new failure | attempt | title\n'
        for stage in results:
            if not stage.get('report_on', True):
                continue
            yield (' {old_failures:10d} | {new_failures:11d} | {attempts:7d}'
                   ' | {title}\n').format(**stage)


class IndustrialTest:
    """Class for running one attempt at an industrial test."""

    @classmethod
    def from_args(cls, env, new_juju_path, stage_attempts, new_agent_url=None,
                  debug=False):
        """Return an IndustrialTest from commandline arguments.

        :param env: The name of the environment to base environments on.
        :param new_juju_path: Path to the "new" (non-system) juju.
        :param new_agent_url: Agent stream url for new client.
        :param stage_attemps: List of stages to attempt.
        :param debug: If True, use juju --debug logging.
        """
        old_env = SimpleEnvironment.from_config(env)
        old_env.environment = env + '-old'
        old_client = EnvJujuClient.by_version(old_env, debug=debug)
        new_env = SimpleEnvironment.from_config(env)
        new_env.environment = env + '-new'
        if new_agent_url is not None:
            new_env.config['tools-metadata-url'] = new_agent_url
        uniquify_local(new_env)
        new_client = EnvJujuClient.by_version(new_env, new_juju_path,
                                              debug=debug)
        return cls(old_client, new_client, stage_attempts)

    def __init__(self, old_client, new_client, stage_attempts):
        """Constructor.

        :param old_client: An EnvJujuClient for the old juju.
        :param new_client: An EnvJujuClient for the new juju.
        :param stage_attemps: List of stages to attempt.
        """
        self.old_client = old_client
        self.new_client = new_client
        self.stage_attempts = stage_attempts

    def run_attempt(self):
        """Perform this attempt, with initial cleanup."""
        self.destroy_both()
        try:
            return list(self.run_stages())
        except CannotUpgradeToOldClient:
            raise
        except Exception as e:
            logging.exception(e)
            self.destroy_both()
            sys.exit(1)

    def destroy_both(self):
        """Destroy the environments of the old and new client."""
        try:
            self.old_client.destroy_environment(delete_jenv=True)
        finally:
            self.new_client.destroy_environment(delete_jenv=True)

    def run_stages(self):
        """Iterator of (boolean, boolean) for stage results.

        Iteration stops when one client has a False result.
        """
        for attempt in self.stage_attempts:
            try:
                for result in attempt.iter_test_results(self.old_client,
                                                        self.new_client):
                    yield result
            except CannotUpgradeToClient as e:
                if e.client is not self.old_client:
                    raise
                raise CannotUpgradeToOldClient(e.client)
            # If a stage ends with a failure, no further stages should be run.
            if False in result[1:]:
                self.destroy_both()
                return


class SteppedStageAttempt:
    """Subclasses of this class implement an industrial test stage with steps.

    Every Stage provides at least one test.  The get_test_info() method
    describes the tests according to their test_id.

    They provide an iter_steps() iterator that acts as a coroutine.  Each test
    has one or more steps, and iter_steps iterates through all the steps of
    every test in the Stage.  For every step, it yields yields a dictionary.
    If the dictionary contains {'result': True}, the test is complete,
    but there may be further tests.  False could be used, but in practise,
    failures are typically handled by raising exceptions.
    """

    @classmethod
    def factory(cls, upgrade_sequence):
        return cls()

    @staticmethod
    def _iter_for_result(iterator):
        """Iterate through an iterator of {'test_id'} with optional result.

        This iterator exists mainly to simplify writing the per-operation
        iterators.

        Each test_id must have at least one {'test_id'}.  The id must not
        change until a result is enountered.
        Convert no-result to None.
        Convert exceptions to a False result.  Exceptions terminate iteration.
        """
        while True:
            last_result = {}
            while 'result' not in last_result:
                try:
                    result = dict(iterator.next())
                except StopIteration:
                    raise
                except CannotUpgradeToClient:
                    raise
                except Exception as e:
                    logging.exception(e)
                    yield{'test_id': last_result.get('test_id'),
                          'result': False}
                    return
                if last_result.get('test_id') is not None:
                    if last_result['test_id'] != result['test_id']:
                        raise ValueError('ID changed without result.')
                if 'result' in result:
                    if last_result == {}:
                        raise ValueError('Result before declaration.')
                else:
                    yield None
                last_result = result
            yield result

    @classmethod
    def _iter_test_results(cls, old_iter, new_iter):
        """Iterate through none-or-result to get result for each operation.

        Yield the result as a tuple of (test-id, old_result, new_result).

        Operations are interleaved between iterators to improve
        responsiveness; an itererator can start a long-running operation,
        yield, then acquire the result of the operation.
        """
        while True:
            old_result = None
            new_result = None
            while None in (old_result, new_result):
                try:
                    if old_result is None:
                        old_result = old_iter.next()
                    if new_result is None:
                        new_result = new_iter.next()
                except StopIteration:
                    return
            if old_result['test_id'] != new_result['test_id']:
                raise ValueError('Test id mismatch.')
            results = (old_result['result'], new_result['result'])
            result_strings = ['succeeded' if r else 'failed' for r in results]
            logging.info('{}: old {}, new {}.'.format(
                cls.get_test_info()[old_result['test_id']]['title'],
                *result_strings))
            yield (old_result['test_id'],) + results

    @classmethod
    def get_test_info(cls):
        """Default implementation uses get_stage_info."""
        return OrderedDict(si.as_tuple() for si in cls.get_stage_info())

    def iter_test_results(self, old, new):
        """Iterate through the results for this operation for both clients."""
        old_iter = self._iter_for_result(self.iter_steps(old))
        new_iter = self._iter_for_result(self.iter_steps(new))
        return self._iter_test_results(old_iter, new_iter)


class BootstrapAttempt(SteppedStageAttempt):
    """Implementation of a bootstrap stage."""

    @staticmethod
    def get_test_info():
        """Describe the tests provided by this Stage."""
        return {'bootstrap': {'title': 'bootstrap'}}

    def iter_steps(self, client):
        """Iterate the steps of this Stage.  See SteppedStageAttempt."""
        results = {'test_id': 'bootstrap'}
        yield results
        with temp_bootstrap_env(
                get_juju_home(), client, set_home=False) as juju_home:
            logging.info('Performing async bootstrap')
            with client.bootstrap_async(juju_home=juju_home):
                yield results
        with wait_for_started(client):
            yield results
        results['result'] = True
        yield results


class CannotUpgradeToClient(Exception):
    """UpgradeJujuAttempt cannot upgrade to the supplied client."""

    def __init__(self, client):
        msg = 'Cannot upgrade to client at "{}"'.format(client.full_path)
        super(CannotUpgradeToClient, self).__init__(msg)
        self.client = client


class CannotUpgradeToOldClient(CannotUpgradeToClient):
    """UpgradeJujuAttempt cannot upgrade to the old client."""


class UpgradeJujuAttempt(SteppedStageAttempt):

    @staticmethod
    def get_test_info():
        return OrderedDict([
            ('prepare-upgrade-juju',
                {'title': 'Prepare upgrade-juju', 'report_on': False}),
            ('upgrade-juju', {'title': 'Upgrade Juju'}),
            ])

    @classmethod
    def factory(cls, upgrade_sequence):
        if len(upgrade_sequence) < 2:
            raise ValueError('Not enough paths for upgrade.')
        bootstrap_paths = dict(
            zip(upgrade_sequence[1:], upgrade_sequence[:-1]))
        return cls(bootstrap_paths)

    def __init__(self, bootstrap_paths):
        super(UpgradeJujuAttempt, self).__init__()
        self.bootstrap_paths = bootstrap_paths

    def iter_steps(self, client):
        ba = BootstrapAttempt()
        try:
            bootstrap_path = self.bootstrap_paths[client.full_path]
        except KeyError:
            raise CannotUpgradeToClient(client)
        bootstrap_client = client.by_version(
            client.env, bootstrap_path, client.debug)
        for result in ba.iter_steps(bootstrap_client):
            result = dict(result)
            result['test_id'] = 'prepare-upgrade-juju'
            yield result
        result = {'test_id': 'upgrade-juju'}
        yield result
        client.upgrade_juju()
        yield result
        client.wait_for_version(client.get_matching_agent_version())
        result['result'] = True
        yield result


class UpgradeCharmAttempt(SteppedStageAttempt):

    prepare = StageInfo('prepare-upgrade-charm', 'Prepare to upgrade charm.',
                        report_on=False)
    upgrade = StageInfo('upgrade-charm', 'Upgrade charm')

    @classmethod
    def get_stage_info(cls):
        return [cls.prepare, cls.upgrade]

    def iter_steps(self, client):
        yield self.prepare.as_result()
        with temp_dir() as temp_repository:
            charm_root = os.path.join(temp_repository, 'trusty', 'mycharm')
            os.makedirs(charm_root)
            with open(os.path.join(charm_root, 'metadata.yaml'), 'w') as f:
                f.write(yaml.safe_dump({
                    'name': 'mycharm',
                    'description': 'foo-description',
                    'summary': 'foo-summary',
                    }))
            client.deploy('local:trusty/mycharm', temp_repository)
            yield self.prepare.as_result()
            client.wait_for_started()
            yield self.prepare.as_result()
            hooks_path = os.path.join(charm_root, 'hooks')
            os.mkdir(hooks_path)
            self.add_hook(hooks_path, 'config-changed', dedent("""\
                #!/bin/sh
                open-port 34
                """))
            self.add_hook(hooks_path, 'upgrade-charm', dedent("""\
                #!/bin/sh
                open-port 42
                """))
            yield self.prepare.as_result(True)
            yield self.upgrade.as_result()
            client.juju(
                'upgrade-charm', ('mycharm', '--repository', temp_repository))
            yield self.upgrade.as_result()
            for status in client.status_until(300):
                ports = status.get_open_ports('mycharm/0')
                if '42/tcp' in ports and '34/tcp' in ports:
                    break
            else:
                raise Exception('42 and/or 34 not opened.')
            yield self.upgrade.as_result(True)

    def add_hook(self, hooks_path, hook_name, hook_contents):
        with open(os.path.join(hooks_path, hook_name), 'w') as f:
            os.fchmod(f.fileno(), 0755)
            f.write(hook_contents)


@contextmanager
def make_substrate_manager(client, required_attrs):
    """A context manager for the client with the required attributes.

    If the substrate cannot be made, or does not have the required attributes,
    return None.  Otherwise, return the substrate.
    """
    with real_make_substrate_manager(client.env.config) as substrate:
        if substrate is not None:
            for attr in required_attrs:
                if getattr(substrate, attr, None) is None:
                    substrate = None
                    break
        yield substrate


class DestroyEnvironmentAttempt(SteppedStageAttempt):
    """Implementation of a destroy-environment stage."""

    @staticmethod
    def get_test_info():
        """Describe the tests provided by this Stage."""
        return OrderedDict([
            ('destroy-env', {'title': 'destroy environment'}),
            ('substrate-clean', {'title': 'check substrate clean'})])

    @classmethod
    def get_security_groups(cls, client):
        with make_substrate_manager(
                client, ['iter_instance_security_groups']) as substrate:
            if substrate is None:
                return
            status = client.get_status()
            instance_ids = [m['instance-id'] for k, m in status.iter_machines()
                            if 'instance-id' in m]
            return dict(substrate.iter_instance_security_groups(instance_ids))

    @classmethod
    def check_security_groups(cls, client, env_groups):
        with make_substrate_manager(
                client, ['iter_instance_security_groups']) as substrate:
            if substrate is None:
                return
            for x in until_timeout(30):
                remain_groups = dict(substrate.iter_security_groups())
                leftovers = set(remain_groups).intersection(env_groups)
                if len(leftovers) == 0:
                    break
        group_text = ', '.join(sorted(remain_groups[l] for l in leftovers))
        if group_text != '':
            raise Exception(
                'Security group(s) not cleaned up: {}.'.format(group_text))

    def iter_steps(cls, client):
        """Iterate the steps of this Stage.  See SteppedStageAttempt."""
        results = {'test_id': 'destroy-env'}
        yield results
        groups = cls.get_security_groups(client)
        client.destroy_environment(force=False)
        # If it hasn't raised an exception, destroy-environment succeeded.
        results['result'] = True
        yield results
        results = {'test_id': 'substrate-clean'}
        yield results
        cls.check_security_groups(client, groups)
        results['result'] = True
        yield results


class EnsureAvailabilityAttempt(SteppedStageAttempt):
    """Implementation of an ensure-availability stage."""

    title = 'ensure-availability -n 3'

    test_id = 'ensure-availability-n3'

    @staticmethod
    def get_test_info():
        return {'ensure-availability-n3': {
            'title': 'ensure-availability -n 3'}}

    def iter_steps(self, client):
        """Iterate the steps of this Stage.  See SteppedStageAttempt."""
        results = {'test_id': 'ensure-availability-n3'}
        yield results
        client.juju('ensure-availability', ('-n', '3'))
        yield results
        client.wait_for_ha()
        results['result'] = True
        yield results


@contextmanager
def wait_until_removed(client, to_remove, timeout=30):
    """Wait until none of the machines are listed in status.

    This is implemented as a context manager so that it is coroutine-friendly.
    The start of the timeout begins at the with statement, but the actual
    waiting (if any) is done when exiting the with block.
    """
    timeout_iter = until_timeout(timeout)
    yield
    to_remove = set(to_remove)
    for ignored in timeout_iter:
        status = client.get_status()
        machines = [k for k, v in status.iter_machines(containers=True) if
                    k in to_remove]
        if machines == []:
            break
    else:
        raise Exception('Timed out waiting for removal')


@contextmanager
def wait_for_started(client):
    """Wait until all agents are listed as started.

    This is implemented as a context manager so that it is coroutine-friendly.
    The start of the timeout begins at the with statement, but the actual
    waiting (if any) is done when exiting the with block.
    """
    timeout_start = datetime.now()
    yield
    client.wait_for_started(start=timeout_start)


class DeployManyAttempt(SteppedStageAttempt):

    @staticmethod
    def get_test_info():
        """Describe the tests provided by this Stage."""
        return OrderedDict([
            ('add-machine-many', {'title': 'add many machines'}),
            ('ensure-machines', {
                'title': 'Ensure sufficient machines', 'report_on': False}),
            ('deploy-many', {'title': 'deploy many'}),
            ('remove-machine-many-lxc', {
                'title': 'remove many machines (lxc)'}),
            ('remove-machine-many-instance', {
                'title': 'remove many machines (instance)'}),
            ])

    def __init__(self, host_count=5, container_count=8):
        super(DeployManyAttempt, self).__init__()
        self.host_count = host_count
        self.container_count = container_count

    def __eq__(self, other):
        if type(self) != type(other):
            return False
        return (self.host_count, self.container_count) == (
            other.host_count, other.container_count)

    def iter_steps(self, client):
        """Iterate the steps of this Stage.  See SteppedStageAttempt."""
        results = {'test_id': 'add-machine-many'}
        yield results
        old_status = client.get_status()
        for machine in range(self.host_count):
            client.juju('add-machine', ())
        timeout_start = datetime.now()
        yield results
        try:
            new_status = client.wait_for_started(start=timeout_start)
        except AgentsNotStarted as e:
            new_status = e.status
            results['result'] = False
        else:
            results['result'] = True
        yield results
        results = {'test_id': 'ensure-machines'}
        yield results
        stuck_new_machines = [
            k for k, v in new_status.iter_new_machines(old_status)
            if v.get('agent-state') != 'started']
        for machine in stuck_new_machines:
            client.juju('destroy-machine', ('--force', machine))
            client.juju('add-machine', ())
        timeout_start = datetime.now()
        yield results
        new_status = client.wait_for_started(start=timeout_start)
        new_machines = dict(new_status.iter_new_machines(old_status))
        if len(new_machines) != self.host_count:
            raise AssertionError('Got {} machines, not {}'.format(
                len(new_machines), self.host_count))
        results['result'] = True
        yield results
        results = {'test_id': 'deploy-many'}
        yield results
        service_names = []
        machine_names = sorted(new_machines, key=int)
        for machine_name in machine_names:
            target = 'lxc:{}'.format(machine_name)
            for container in range(self.container_count):
                service = 'ubuntu{}x{}'.format(machine_name, container)
                client.juju('deploy', ('--to', target, 'ubuntu', service))
                service_names.append(service)
        timeout_start = datetime.now()
        yield results
        status = client.wait_for_started(start=timeout_start)
        results['result'] = True
        yield results
        results = {'test_id': 'remove-machine-many-lxc'}
        yield results
        services = [status.status['services'][key] for key in service_names]
        lxc_machines = set()
        for service in services:
            for unit in service['units'].values():
                lxc_machines.add(unit['machine'])
                client.juju('remove-machine', ('--force', unit['machine']))
        with wait_until_removed(client, lxc_machines):
            yield results
        results['result'] = True
        yield results
        results = {'test_id': 'remove-machine-many-instance'}
        yield results
        for machine_name in machine_names:
            client.juju('remove-machine', (machine_name,))
        with wait_until_removed(client, machine_names):
            yield results
        results['result'] = True
        yield results


class BackupRestoreAttempt(SteppedStageAttempt):

    @staticmethod
    def get_test_info():
        """Describe the tests provided by this Stage."""
        return {'back-up-restore': {'title': 'Back-up / restore'}}

    def iter_steps(cls, client):
        """Iterate the steps of this Stage.  See SteppedStageAttempt."""
        results = {'test_id': 'back-up-restore'}
        yield results
        backup_file = client.backup()
        try:
            status = client.get_status()
            instance_id = status.get_instance_id('0')
            host = get_machine_dns_name(client, 0)
            terminate_instances(client.env, [instance_id])
            yield results
            wait_for_state_server_to_shutdown(host, client, instance_id)
            yield results
            with client.juju_async('restore', (backup_file,)):
                yield results
        finally:
            os.unlink(backup_file)
        with wait_for_started(client):
            yield results
        results['result'] = True
        yield results


suites = {
    QUICK: (BootstrapAttempt, DestroyEnvironmentAttempt),
    DENSITY: (BootstrapAttempt, DeployManyAttempt,
              DestroyEnvironmentAttempt),
    FULL: (BootstrapAttempt, UpgradeCharmAttempt, DeployManyAttempt,
           BackupRestoreAttempt, EnsureAvailabilityAttempt,
           DestroyEnvironmentAttempt),
    BACKUP: (BootstrapAttempt, BackupRestoreAttempt,
             DestroyEnvironmentAttempt),
    UPGRADE: (UpgradeJujuAttempt, DestroyEnvironmentAttempt),
    }


def suite_list(suite_str):
    suite_list = suite_str.split(',')
    for suite in suite_list:
        if suite not in suites:
            sys.stderr.write(
                "Invalid argument suite: invalid choice: '{}'\n".format(suite))
            sys.exit(1)
    return suite_list


def parse_args(args=None):
    """Parse commandline arguments into a Namespace."""
    parser = ArgumentParser()
    parser.add_argument('env')
    parser.add_argument('new_juju_path')
    parser.add_argument('suite', type=suite_list)
    parser.add_argument('--attempts', type=int, default=2)
    parser.add_argument('--json-file')
    parser.add_argument('--new-agent-url')
    parser.add_argument('--single', action='store_true')
    parser.add_argument('--debug', action='store_true', default=False)
    parser.add_argument(
        '--old-stable', help='Path to a version of juju that stable can'
        ' upgrade from.')
    return parser.parse_args(args)


def maybe_write_json(filename, results):
    if filename is None:
        return
    with open(filename, 'w') as json_file:
        json.dump(results, json_file, indent=2)


def run_single(args):
    env = SimpleEnvironment.from_config(args.env)
    env.environment = env.environment + '-single'
    upgrade_client = EnvJujuClient.by_version(
        env, debug=args.debug)
    client = EnvJujuClient.by_version(
        env,  args.new_juju_path, debug=args.debug)
    client.destroy_environment()
    for suite in args.suite:
        stages = MultiIndustrialTest.get_stages(suite, env.config)
        upgrade_sequence = [upgrade_client.full_path, client.full_path]
        try:
            for stage in stages:
                for step in stage.factory(upgrade_sequence).iter_steps(client):
                    print step
        except BaseException as e:
            logging.exception(e)
            client.destroy_environment()


def main():
    configure_logging(logging.INFO)
    args = parse_args()
    if args.single:
        run_single(args)
        return
    results_list = []
    for suite in args.suite:
        mit = MultiIndustrialTest.from_args(args, suite)
        try:
            results_list.append(mit.run_tests())
        except CannotUpgradeToOldClient:
            if args.old_stable is not None:
                raise
            sys.stderr.write('Upgade tests require --old-stable.\n')
            sys.exit(1)
    results = MultiIndustrialTest.combine_results(results_list)
    maybe_write_json(args.json_file, results)
    sys.stdout.writelines(mit.results_table(results['results']))


if __name__ == '__main__':
    main()
