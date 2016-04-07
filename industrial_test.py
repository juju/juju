#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
from collections import OrderedDict
from contextlib import contextmanager
from datetime import datetime
from itertools import count
import json
import logging
import os
import sys
from textwrap import dedent

import yaml

from deploy_stack import (
    BootstrapManager,
    wait_for_state_server_to_shutdown,
    )
from jujupy import (
    AgentsNotStarted,
    EnvJujuClient,
    get_machine_dns_name,
    LXC_MACHINE,
    LXD_MACHINE,
    SimpleEnvironment,
    uniquify_local,
    )
from substrate import (
    make_substrate_manager as real_make_substrate_manager,
    terminate_instances,
    )
from utility import (
    configure_logging,
    LoggedException,
    temp_dir,
    until_timeout,
    )


__metaclass__ = type


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
    :ivar agent_stream: The agent stream to use for testing.
    """

    @classmethod
    def from_args(cls, args, suite):
        stages = suites[suite]
        return cls(args.env, args.new_juju_path,
                   stages, args.log_dir, args.attempts, args.attempts * 2,
                   args.new_agent_url, args.debug, args.old_stable,
                   args.agent_stream)

    def __init__(self, env, new_juju_path, stages, log_dir, attempt_count=2,
                 max_attempts=1, new_agent_url=None, debug=False,
                 really_old_path=None, agent_stream=None):
        self.env = env
        self.really_old_path = really_old_path
        self.new_juju_path = new_juju_path
        self.new_agent_url = new_agent_url
        self.stages = stages
        self.attempt_count = attempt_count
        self.max_attempts = max_attempts
        self.debug = debug
        self.log_parent_dir = log_dir
        self.agent_stream = agent_stream

    def make_results(self):
        """Return a results list for use in run_tests."""
        results = []
        for test_id, info in self.stages.get_test_info().items():
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
        stage_attempts = [self.stages.factory(upgrade_sequence,
                                              self.log_parent_dir,
                                              self.agent_stream)]
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
        old_env.set_model_name(env + '-old')
        old_client = EnvJujuClient.by_version(old_env, debug=debug)
        new_env = SimpleEnvironment.from_config(env)
        new_env.set_model_name(env + '-new')
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
        try:
            return list(self.run_stages())
        except CannotUpgradeToOldClient:
            raise
        except Exception as e:
            logging.exception(e)
            sys.exit(1)

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
            # If a stage ends with a failure, no further stages should
            # be run.
            if False in result[1:]:
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
    def factory(cls, upgrade_sequence, agent_stream):
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
        Iterators will always be closed when _iter_for_result is finished or
        closed.
        """
        try:
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
                    except LoggedException:
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
        finally:
            iterator.close()

    def _iter_test_results(self, old_iter, new_iter):
        """Iterate through none-or-result to get result for each operation.

        Yield the result as a tuple of (test-id, old_result, new_result).

        Operations are interleaved between iterators to improve
        responsiveness; an itererator can start a long-running operation,
        yield, then acquire the result of the operation.
        """
        try:
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
                    raise ValueError('Test id mismatch: {} {}'.format(
                        old_result['test_id'], new_result['test_id']))
                results = (old_result['result'], new_result['result'])
                result_strings = ['succeeded' if r else 'failed'
                                  for r in results]
                logging.info('{}: old {}, new {}.'.format(
                    self.get_test_info()[old_result['test_id']]['title'],
                    *result_strings))
                yield (old_result['test_id'],) + results
        except CannotUpgradeToClient:
            raise
        except Exception as e:
            logging.exception(e)
            raise LoggedException(e)
        finally:
            # Shut down both iterators, including destroy-environment
            try:
                old_iter.close()
            finally:
                new_iter.close()

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

    def get_bootstrap_client(self, client):
        return client

    def iter_steps(self, client):
        """Iterate the steps of this Stage.  See SteppedStageAttempt."""
        results = {'test_id': 'bootstrap'}
        yield results
        logging.info('Performing async bootstrap')
        with client.bootstrap_async():
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


class PrepareUpgradeJujuAttempt(SteppedStageAttempt):
    """Prepare to run an UpgradeJujuAttempt.  This is the bootstrap portion."""

    prepare_upgrade = StageInfo(
        'prepare-upgrade-juju',
        'Prepare upgrade-juju',
        report_on=False,
        )

    @classmethod
    def get_test_info(cls):
        return dict([cls.prepare_upgrade.as_tuple()])

    @classmethod
    def factory(cls, upgrade_sequence, agent_stream):
        if len(upgrade_sequence) < 2:
            raise ValueError('Not enough paths for upgrade.')
        bootstrap_paths = dict(
            zip(upgrade_sequence[1:], upgrade_sequence[:-1]))
        return cls(bootstrap_paths)

    def __init__(self, bootstrap_paths):
        super(PrepareUpgradeJujuAttempt, self).__init__()
        self.bootstrap_paths = bootstrap_paths

    def get_bootstrap_client(self, client):
        """Return a client to be used for bootstrapping.

        Because we intend to upgrade, we produce a client that is older than
        the supplied client.  In a correct upgrade_sequence, the path for
        older clients come before the paths for newer clients.
        """
        try:
            bootstrap_path = self.bootstrap_paths[client.full_path]
        except KeyError:
            raise CannotUpgradeToClient(client)
        return client.by_version(
            client.env, bootstrap_path, client.debug)

    def iter_steps(self, client):
        """Use a BootstrapAttempt with a different client."""
        ba = BootstrapAttempt()
        bootstrap_client = self.get_bootstrap_client(client)
        for result in ba.iter_steps(bootstrap_client):
            result = dict(result)
            result['test_id'] = self.prepare_upgrade.stage_id
            yield result


class UpgradeJujuAttempt(SteppedStageAttempt):
    """Perform an 'upgrade-juju' on the environment."""

    @staticmethod
    def get_test_info():
        return OrderedDict([(
            'upgrade-juju', {'title': 'Upgrade Juju'}),
            ])

    def iter_steps(self, client):
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
            os.fchmod(f.fileno(), 0o755)
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

    destroy = StageInfo('destroy-env', 'destroy environment')
    substrate_clean = StageInfo('substrate-clean', 'check substrate clean')

    @classmethod
    def get_stage_info(cls):
        return [cls.destroy, cls.substrate_clean]

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
        yield cls.destroy.as_result()
        groups = cls.get_security_groups(client)
        if client.is_jes_enabled():
            client.kill_controller()
        elif client.destroy_environment(force=False) != 0:
            yield cls.destroy.as_result(False)
            return
        yield cls.destroy.as_result(True)
        yield cls.substrate_clean.as_result()
        cls.check_security_groups(client, groups)
        yield cls.substrate_clean.as_result(True)


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
        client.enable_ha()
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
            ('remove-machine-many-container', {
                'title': 'remove many machines (container)'}),
            ('remove-machine-many-instance', {
                'title': 'remove many machines (instance)'}),
            ])

    def __init__(self, host_count=1, container_count=1):
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
            client.juju('remove-machine', ('--force', machine))
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
        machine_type = client.preferred_container()
        for machine_name in machine_names:
            target = '{}:{}'.format(machine_type, machine_name)
            for container in range(self.container_count):
                service = 'ubuntu{}x{}'.format(machine_name, container)
                client.juju('deploy', ('--to', target, 'ubuntu', service))
                service_names.append(service)
        timeout_start = datetime.now()
        yield results
        status = client.wait_for_started(start=timeout_start)
        results['result'] = True
        yield results
        results = {'test_id': 'remove-machine-many-container'}
        yield results
        services = [status.status['services'][key] for key in service_names]
        container_machines = set()
        for service in services:
            for unit in service['units'].values():
                container_machines.add(unit['machine'])
                client.juju('remove-machine', ('--force', unit['machine']))
        with wait_until_removed(client, container_machines):
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
            host = get_machine_dns_name(client, '0')
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


class AttemptSuiteFactory:
    """Factory to produce AttemptSuite objects and backing data.

    :ivar bootstrap_attempt: AttemptSuite class to use for bootstrap.
    :ivar attempt_list: List of SteppedStageAttempts to perform.
    """

    def __init__(self, attempt_list, bootstrap_attempt=None):
        if bootstrap_attempt is None:
            bootstrap_attempt = BootstrapAttempt
        self.bootstrap_attempt = bootstrap_attempt
        self.attempt_list = attempt_list

    prepare_suite = StageInfo('prepare-suite', 'Prepare suite tests',
                              report_on=False)

    def __eq__(self, other):
        if type(self) != type(other):
            return False
        elif self.bootstrap_attempt != other.bootstrap_attempt:
            return False
        elif self.attempt_list != other.attempt_list:
            return False
        else:
            return True

    def get_test_info(self):
        """Describe the tests provided by this factory."""
        result = OrderedDict(self.bootstrap_attempt.get_test_info())
        result.update([self.prepare_suite.as_tuple()])
        for attempt in self.attempt_list:
            result.update(attempt.get_test_info())
        result.update(DestroyEnvironmentAttempt.get_test_info())
        return result

    def factory(self, upgrade_sequence, log_dir, agent_stream):
        """Emit an AttemptSuite.

        :param upgrade_sequence: The sequence of jujus to upgrade, for
            UpgradeJujuAttempt.
        :param log_dir: Directory to store logs and other artifacts in.
        :param agent_stream: The agent stream to use for testing.
        """
        return AttemptSuite(self, upgrade_sequence, log_dir, agent_stream)


class AttemptSuite(SteppedStageAttempt):
    """A SteppedStageAttempt that runs other SteppedStageAttempts.

    :ivar attempt_list: An AttemptSuiteFactory with the list of
        SteppedStageAttempts to run.
    :ivar upgrade_sequence: The sequence of jujus to upgrade, for
        UpgradeJujuAttempt.
    :ivar log_dir: Directory to store logs and other artifacts in.
    :ivar agent_stream: The agent stream to use for testing.
    """

    def __init__(self, attempt_list, upgrade_sequence, log_dir, agent_stream):
        self.attempt_list = attempt_list
        self.upgrade_sequence = upgrade_sequence
        self.log_dir = log_dir
        self.agent_stream = agent_stream

    def get_test_info(self):
        """Describe the tests provided by this Stage."""
        return self.attempt_list.get_test_info()

    def iter_steps(self, client):
        """Iterate through the steps of attempt_list.

        Create a BootstrapManager.  First bootstrap in bootstrap_context using
        attempt_list.bootstrap_attempt.  Then run the other
        SteppedStageAttempts in runtime_context.  If any of this fails,
        BootstrapManager will automatically tear down.  Otherwise, iterate
        through DestroyEnvironmentAttempt.

        The actual generator is _iter_bs_manager_steps, to simplify testing.
        """
        bootstrap_attempt = self.attempt_list.bootstrap_attempt.factory(
            self.upgrade_sequence, self.agent_stream)
        bs_client = bootstrap_attempt.get_bootstrap_client(client)
        bs_jes_enabled = bs_client.is_jes_enabled()
        jes_enabled = client.is_jes_enabled()
        bs_manager = BootstrapManager(
            client.env.environment, bs_client, bs_client,
            bootstrap_host=None,
            machines=[], series=None, agent_url=None,
            agent_stream=self.agent_stream, region=None,
            log_dir=make_log_dir(self.log_dir), keep_env=True,
            permanent=jes_enabled, jes_enabled=bs_jes_enabled)
        return self._iter_bs_manager_steps(bs_manager, client,
                                           bootstrap_attempt, jes_enabled)

    def _iter_bs_manager_steps(self, bs_manager, client, bootstrap_attempt,
                               jes_enabled):
        with bs_manager.top_context() as machines:
            with bs_manager.bootstrap_context(machines):
                for result in bootstrap_attempt.iter_steps(client):
                    yield result
            if result['result'] is False:
                return
            yield self.attempt_list.prepare_suite.as_result()
            with bs_manager.runtime_context(machines):
                # Switch from bootstrap client to real client, in case test
                # steps (i.e. upgrade) make bs_client unable to tear down.
                bs_manager.client = client
                bs_manager.tear_down_client = client
                bs_manager.jes_enabled = jes_enabled
                attempts = [
                    a.factory(self.upgrade_sequence, self.agent_stream)
                    for a in self.attempt_list.attempt_list]
                yield self.attempt_list.prepare_suite.as_result(True)
                for attempt in attempts:
                    for result in attempt.iter_steps(client):
                        yield result
                    # If the last step of a SteppedStageAttempt is False, stop
                    if result['result'] is False:
                        return
                # We don't want BootstrapManager.tear_down to run-- we want
                # DesstroyEnvironmentAttempt.  But we do need BootstrapManager
                # to finish up before we run DestroyEnvironmentAttempt.
                bs_manager.keep_env = True
            try:
                for result in DestroyEnvironmentAttempt().iter_steps(client):
                    yield result
            except:
                bs_manager.tear_down()
                raise
            finally:
                bs_manager.keep_env = False


suites = {
    QUICK: AttemptSuiteFactory([]),
    DENSITY: AttemptSuiteFactory([DeployManyAttempt]),
    FULL: AttemptSuiteFactory([
        UpgradeCharmAttempt, DeployManyAttempt, BackupRestoreAttempt,
        EnsureAvailabilityAttempt]),
    BACKUP: AttemptSuiteFactory([BackupRestoreAttempt]),
    UPGRADE: AttemptSuiteFactory(
        [UpgradeJujuAttempt], bootstrap_attempt=PrepareUpgradeJujuAttempt),
    }


def suite_list(suite_str):
    suite_list = suite_str.split(',')
    for suite in suite_list:
        if suite not in suites:
            sys.stderr.write(
                "Invalid argument suite: invalid choice: '{}'\n".format(suite))
            sys.exit(1)
    return suite_list


_log_dir_count = count()


def make_log_dir(log_parent_dir):
    """Make a numbered directory for logs."""
    new_log_dir = os.path.join(log_parent_dir, str(_log_dir_count.next()))
    os.mkdir(new_log_dir)
    return new_log_dir


def parse_args(args=None):
    """Parse commandline arguments into a Namespace."""
    parser = ArgumentParser()
    parser.add_argument('env')
    parser.add_argument('new_juju_path')
    parser.add_argument('suite', type=suite_list)
    parser.add_argument('log_dir',
                        help='directory for logs and other artifacts.')
    parser.add_argument('--attempts', type=int, default=2)
    parser.add_argument('--json-file')
    parser.add_argument('--new-agent-url')
    parser.add_argument('--single', action='store_true')
    parser.add_argument('--debug', action='store_true', default=False)
    parser.add_argument('--agent-stream',
                        help='Agent stream to use for tests.')
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
    env.set_model_name(env.environment + '-single')
    upgrade_client = EnvJujuClient.by_version(
        env, debug=args.debug)
    client = EnvJujuClient.by_version(
        env,  args.new_juju_path, debug=args.debug)
    for suite in args.suite:
        factory = suites[suite]
        upgrade_sequence = [upgrade_client.full_path, client.full_path]
        suite = factory.factory(upgrade_sequence, args.log_dir,
                                args.agent_stream)
        steps_iter = suite.iter_steps(client)
        for step in steps_iter:
            print(step)


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
        except LoggedException:
            continue
    results = MultiIndustrialTest.combine_results(results_list)
    maybe_write_json(args.json_file, results)
    sys.stdout.writelines(mit.results_table(results['results']))


if __name__ == '__main__':
    main()
