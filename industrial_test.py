#!/usr/bin/env python
__metaclass__ = type

from argparse import ArgumentParser
from collections import OrderedDict
import json
import logging
import sys

from deploy_stack import (
    get_machine_dns_name,
    wait_for_state_server_to_shutdown,
    )
from jujuconfig import get_juju_home
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    temp_bootstrap_env,
    uniquify_local,
    )
from substrate import (
    make_substrate,
    terminate_instances,
    )
from utility import until_timeout


class MultiIndustrialTest:
    """Run IndustrialTests until desired number of results are achieved.

    :ivar env: The name of the environment to use as a base.
    :ivar new_juju_path: Path to the non-system juju.
    :ivar stages: A list of StageAttempts.
    :ivar attempt_count: The number of attempts needed for each stage.
    """

    @classmethod
    def from_args(cls, args):
        if args.quick:
            stages = [BootstrapAttempt, DeployManyAttempt,
                      DestroyEnvironmentAttempt]
        else:
            stages = [BootstrapAttempt, DeployManyAttempt,
                      BackupRestoreAttempt, EnsureAvailabilityAttempt,
                      DestroyEnvironmentAttempt]
        return cls(args.env, args.new_juju_path,
                   stages, args.attempts, args.attempts * 2,
                   args.new_agent_url)

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
            self.old_client.destroy_environment()
        finally:
            self.new_client.destroy_environment()

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

    @staticmethod
    def _iter_test_results(old_iter, new_iter):
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
            yield (old_result['test_id'], old_result['result'],
                   new_result['result'])

    def iter_test_results(self, old, new):
        """Iterate through the results for this operation for both clients."""
        old_iter = self._iter_for_result(self.iter_steps(old))
        new_iter = self._iter_for_result(self.iter_steps(new))
        return self._iter_test_results(old_iter, new_iter)


class BootstrapAttempt(StageAttempt):
    """Implementation of a bootstrap stage."""

    title = 'bootstrap'

    test_id = 'bootstrap'

    def _operation(self, client):
        with temp_bootstrap_env(get_juju_home(), client):
            client.bootstrap()

    def _result(self, client):
        client.wait_for_started()
        return True


class DestroyEnvironmentAttempt(SteppedStageAttempt):
    """Implementation of a destroy-environment stage."""

    @staticmethod
    def get_test_info():
        return OrderedDict([
            ('destroy-env', {'title': 'destroy environment'}),
            ('substrate-clean', {'title': 'check substrate clean'})])

    @classmethod
    def get_security_groups(cls, client):
        substrate = make_substrate(client.env.config)
        if substrate is None:
            return
        status = client.get_status()
        instance_ids = [m['instance-id'] for k, m in status.iter_machines()
                        if 'instance-id' in m]
        return dict(substrate.iter_instance_security_groups(instance_ids))

    @classmethod
    def check_security_groups(cls, client, env_groups):
        substrate = make_substrate(client.env.config)
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
        results = {'test_id': 'destroy-env'}
        yield results
        groups = cls.get_security_groups(client)
        client.juju('destroy-environment', ('-y', client.env.environment),
                    include_e=False)
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


class DeployManyAttempt(SteppedStageAttempt):

    @staticmethod
    def get_test_info():
        return OrderedDict([
            ('add-machine-many', {'title': 'add many machines'}),
            ('deploy-many', {'title': 'deploy many'}),
            ])

    def __init__(self, host_count=5, container_count=10):
        super(DeployManyAttempt, self).__init__()
        self.host_count = host_count
        self.container_count = container_count

    def iter_steps(self, client):
        results = {'test_id': 'add-machine-many'}
        yield results
        old_status = client.get_status()
        for machine in range(self.host_count):
            client.juju('add-machine', ())
        yield results
        new_status = client.wait_for_started()
        results['result'] = True
        yield results
        results = {'test_id': 'deploy-many'}
        yield results
        new_machines = dict(new_status.iter_new_machines(old_status))
        if len(new_machines) != self.host_count:
            raise AssertionError('Got {} machines, not {}'.format(
                len(new_machines), self.host_count))
        for machine_name in sorted(new_machines, key=int):
            target = 'lxc:{}'.format(machine_name)
            for container in range(self.container_count):
                service = 'ubuntu{}x{}'.format(machine_name, container)
                client.juju('deploy', ('--to', target, 'ubuntu', service))
        yield results
        client.wait_for_started()
        results['result'] = True
        yield results


class BackupRestoreAttempt(StageAttempt):

    title = 'Back-up / restore'

    test_id = 'back-up-restore'

    def _operation(self, client):
        backup_file = client.backup()
        status = client.get_status()
        instance_id = status.get_instance_id('0')
        host = get_machine_dns_name(client, 0)
        terminate_instances(client.env, [instance_id])
        wait_for_state_server_to_shutdown(host, client, instance_id)
        client.juju('restore', (backup_file,))

    def _result(self, client):
        client.wait_for_started()
        return True


def parse_args(args=None):
    """Parse commandline arguments into a Namespace."""
    parser = ArgumentParser()
    parser.add_argument('env')
    parser.add_argument('new_juju_path')
    parser.add_argument('--attempts', type=int, default=2)
    parser.add_argument('--json-file')
    parser.add_argument('--quick', action='store_true')
    parser.add_argument('--new-agent-url')
    return parser.parse_args(args)


def maybe_write_json(filename, results):
    if filename is None:
        return
    with open(filename, 'w') as json_file:
        json.dump(results, json_file, indent=2)


def main():
    args = parse_args()
    mit = MultiIndustrialTest.from_args(args)
    results = mit.run_tests()
    maybe_write_json(args.json_file, results)
    sys.stdout.writelines(mit.results_table(results['results']))


if __name__ == '__main__':
    main()
