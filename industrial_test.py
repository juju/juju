#!/usr/bin/env python
__metaclass__ = type

from argparse import ArgumentParser
from collections import OrderedDict
from contextlib import contextmanager
from datetime import datetime
import json
import logging
import sys

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
    make_substrate as real_make_substrate,
    terminate_instances,
    )
from utility import (
    configure_logging,
    until_timeout,
    )


QUICK = 'quick'
DENSITY = 'density'
FULL = 'full'
BACKUP = 'backup'


class MultiIndustrialTest:
    """Run IndustrialTests until desired number of results are achieved.

    :ivar env: The name of the environment to use as a base.
    :ivar new_juju_path: Path to the non-system juju.
    :ivar stages: A list of StageAttempts.
    :ivar attempt_count: The number of attempts needed for each stage.
    """

    @classmethod
    def from_args(cls, args):
        config = SimpleEnvironment.from_config(args.env).config
        stages = cls.get_stages(args.suite, config)
        return cls(args.env, args.new_juju_path,
                   stages, args.attempts, args.attempts * 2,
                   args.new_agent_url)

    @staticmethod
    def get_stages(suite, config):
        stages = list(suites[suite])
        if config.get('type') == 'maas':
            stages = [
                DeployManyFactory(2, 2) if s is DeployManyAttempt else s
                for s in stages]
        return stages

    def __init__(self, env, new_juju_path, stages, attempt_count=2,
                 max_attempts=1, new_agent_url=None):
        self.env = env
        self.new_juju_path = new_juju_path
        self.new_agent_url = new_agent_url
        self.stages = stages
        self.attempt_count = attempt_count
        self.max_attempts = max_attempts

    def make_results(self):
        """Return a results list for use in run_tests."""
        results = []
        for stage in self.stages:
            for test_id, info in stage.get_test_info().items():
                results.append({
                    'title': info['title'],
                    'test_id': test_id,
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

    def make_industrial_test(self):
        """Create an IndustrialTest for this MultiIndustrialTest."""
        stage_attempts = [stage() for stage in self.stages]
        return IndustrialTest.from_args(self.env, self.new_juju_path,
                                        stage_attempts, self.new_agent_url)

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
            yield (' {old_failures:10d} | {new_failures:11d} | {attempts:7d}'
                   ' | {title}\n').format(**stage)


class IndustrialTest:
    """Class for running one attempt at an industrial test."""

    @classmethod
    def from_args(cls, env, new_juju_path, stage_attempts, new_agent_url=None):
        """Return an IndustrialTest from commandline arguments.

        :param env: The name of the environment to base environments on.
        :param new_juju_path: Path to the "new" (non-system) juju.
        :param new_agent_url: Agent stream url for new client.
        :param stage_attemps: List of stages to attempt.
        """
        old_env = SimpleEnvironment.from_config(env)
        old_env.environment = env + '-old'
        old_client = EnvJujuClient.by_version(old_env)
        new_env = SimpleEnvironment.from_config(env)
        new_env.environment = env + '-new'
        if new_agent_url is not None:
            new_env.config['tools-metadata-url'] = new_agent_url
        uniquify_local(new_env)
        new_client = EnvJujuClient.by_version(new_env, new_juju_path)
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
            for result in attempt.iter_test_results(self.old_client,
                                                    self.new_client):
                yield result
            # If a stage ends with a failure, no further stages should be run.
            if False in result[1:]:
                self.destroy_both()
                return


class StageAttempt:
    """Attempt to run a testing stage."""

    def __init__(self):
        self.failure_clients = set()

    @classmethod
    def get_test_info(cls):
        """Describe the tests provided by this Stage."""
        return {cls.test_id: {'title': cls.title}}

    def do_stage(self, old, new):
        """Do this stage, return a tuple.

        This method may be overridden, but it is more typical to provide
        do_operation and get_result.
        :param old: The old juju client.
        :param new: The new juju client.
        :return: a tuple of (old_succeeded, new_succeeded).
        """
        self.do_operation(old)
        self.do_operation(new)
        old_result = self.get_result(old)
        new_result = self.get_result(new)
        return old_result, new_result

    def iter_test_results(self, old, new):
        old_result, new_result = self.do_stage(old, new)
        yield self.test_id, old_result, new_result

    def do_operation(self, client, output=None):
        """Perform this stage's operation.

        This implementation requires a subclass to declare _operation.
        Exceptions raised by _operation are logged and cause the operation to
        be considered failed for that client.
        """
        try:
            self._operation(client)
        except Exception as e:
            logging.exception(e)
            self.failure_clients.add(client)

    def get_result(self, client):
        """Determine whether this stage's operation succeeded.

        This implementation requires a subclass to declare _result.
        If _operation failed for this, this returns False.
        If _result raises an exception, this returns False.
        Otherwise, this returns the value of get_result.
        """
        if client in self.failure_clients:
            return False
        try:
            return self._result(client)
        except Exception as e:
            logging.exception(e)
            return False


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


def make_substrate(client, required_attrs):
    """Make a substrate for the client with the required attributes.

    If the substrate cannot be made, or does not have the required attributes,
    return None.  Otherwise, return the substrate.
    """
    substrate = real_make_substrate(client.env.config)
    if substrate is None:
        return None
    for attr in required_attrs:
        if getattr(substrate, attr, None) is None:
            return None
    return substrate


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
        substrate = make_substrate(
            client, ['iter_instance_security_groups'])
        if substrate is None:
            return
        status = client.get_status()
        instance_ids = [m['instance-id'] for k, m in status.iter_machines()
                        if 'instance-id' in m]
        return dict(substrate.iter_instance_security_groups(instance_ids))

    @classmethod
    def check_security_groups(cls, client, env_groups):
        substrate = make_substrate(
            client, ['iter_instance_security_groups'])
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


class EnsureAvailabilityAttempt(StageAttempt):
    """Implementation of an ensure-availability stage."""

    title = 'ensure-availability -n 3'

    test_id = 'ensure-availability-n3'

    def _operation(self, client):
        client.juju('ensure-availability', ('-n', '3'))

    def _result(self, client):
        client.wait_for_ha()
        return True


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
            ('ensure-machines', {'title': 'Ensure sufficient machines'}),
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


class DeployManyFactory:
    """Factory delivering pre-configured DeployManyAttempts.

    :ivar host_count: The number of hosts the DeployManyAttempts should
        attempt to deploy.
    :ivar container_count: The number of containers the DeployManyAttempts
        should attempt to deploy.
    """

    def __init__(self, host_count, container_count):
        self.host_count = host_count
        self.container_count = container_count

    def __eq__(self, other):
        if type(self) != type(other):
            return False
        return (self.host_count, self.container_count) == (
            other.host_count, other.container_count)

    @staticmethod
    def get_test_info():
        """Describe the tests provided by DeployManyAttempt."""
        return DeployManyAttempt.get_test_info()

    def __call__(self):
        return DeployManyAttempt(self.host_count, self.container_count)


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
        status = client.get_status()
        instance_id = status.get_instance_id('0')
        host = get_machine_dns_name(client, 0)
        terminate_instances(client.env, [instance_id])
        yield results
        wait_for_state_server_to_shutdown(host, client, instance_id)
        yield results
        client.juju('restore', (backup_file,))
        with wait_for_started(client):
            yield results
        results['result'] = True
        yield results


suites = {
    QUICK: (BootstrapAttempt, DestroyEnvironmentAttempt),
    DENSITY: (BootstrapAttempt, DeployManyAttempt,
              DestroyEnvironmentAttempt),
    FULL: (BootstrapAttempt, DeployManyAttempt,
           BackupRestoreAttempt, EnsureAvailabilityAttempt,
           DestroyEnvironmentAttempt),
    BACKUP: (BootstrapAttempt, BackupRestoreAttempt,
             DestroyEnvironmentAttempt),
    }


def parse_args(args=None):
    """Parse commandline arguments into a Namespace."""
    parser = ArgumentParser()
    parser.add_argument('env')
    parser.add_argument('new_juju_path')
    parser.add_argument('suite', choices=suites.keys())
    parser.add_argument('--attempts', type=int, default=2)
    parser.add_argument('--json-file')
    parser.add_argument('--new-agent-url')
    return parser.parse_args(args)


def maybe_write_json(filename, results):
    if filename is None:
        return
    with open(filename, 'w') as json_file:
        json.dump(results, json_file, indent=2)


def main():
    configure_logging(logging.INFO)
    args = parse_args()
    mit = MultiIndustrialTest.from_args(args)
    results = mit.run_tests()
    maybe_write_json(args.json_file, results)
    sys.stdout.writelines(mit.results_table(results['results']))


if __name__ == '__main__':
    main()
