#!/usr/bin/env python
__metaclass__ = type

from argparse import ArgumentParser
import logging
import sys

from jujuconfig import get_juju_home
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    temp_bootstrap_env,
    uniquify_local,
    )


class MultiIndustrialTest:
    """Run IndustrialTests until desired number of results are achieved.

    :ivar env: The name of the environment to use as a base.
    :ivar new_juju_path: Path to the non-system juju.
    :ivar stages: A list of StageAttempts.
    :ivar attempt_count: The number of attempts needed for each stage.
    """

    def __init__(self, env, new_juju_path, stages, attempt_count=2,
                 max_attempts=1):
        self.env = env
        self.new_juju_path = new_juju_path
        self.stages = stages
        self.attempt_count = attempt_count
        self.max_attempts = max_attempts

    def make_results(self):
        """Return a results list for use in run_tests."""
        return [{
            'title': stage.title,
            'attempts': 0,
            'old_failures': 0,
            'new_failures': 0,
        } for stage in self.stages]

    def run_tests(self):
        """Run all stages until required number of attempts are achieved.

        :return: a list of dicts describing output.
        """
        results = self.make_results()
        for unused_ in range(self.max_attempts):
            if results[-1]['attempts'] >= self.attempt_count:
                break
            industrial = self.make_industrial_test()
            self.update_results(industrial.run_attempt(), results)
        return results

    def make_industrial_test(self):
        """Create an IndustrialTest for this MultiIndustrialTest."""
        stage_attempts = [stage() for stage in self.stages]
        return IndustrialTest.from_args(self.env, self.new_juju_path,
                                        stage_attempts)

    def update_results(self, run_attempt, results):
        """Update results with data from run_attempt.

        Results for stages that have already reached self.attempts are
        ignored.
        """
        for result, cur_result in zip(results, run_attempt):
            if result['attempts'] >= self.attempt_count:
                continue
            result['attempts'] += 1
            if not cur_result[0]:
                result['old_failures'] += 1
            if not cur_result[1]:
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
    def from_args(cls, env, new_juju_path, stage_attempts):
        """Return an IndustrialTest from commandline arguments.

        :param env: The name of the environment to base environments on.
        :param new_juju_path: Path to the "new" (non-system) juju.
        :param stage_attemps: List of stages to attempt.
        """
        old_env = SimpleEnvironment.from_config(env)
        old_env.environment = env + '-old'
        old_client = EnvJujuClient.by_version(old_env)
        new_env = SimpleEnvironment.from_config(env)
        new_env.environment = env + '-new'
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
        self.old_client.destroy_environment()
        self.new_client.destroy_environment()
        return self.run_stages()

    def run_stages(self):
        """Iterator of (boolean, boolean) for stage results.

        Iteration stops when one client has a False result.
        """
        for attempt in self.stage_attempts:
            result = attempt.do_stage(self.old_client, self.new_client)
            yield result
            if False in result:
                try:
                    self.old_client.destroy_environment()
                finally:
                    self.new_client.destroy_environment()
                break


class StageAttempt:
    """Attempt to run a testing stage."""

    def __init__(self):
        self.failure_clients = set()

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


class BootstrapAttempt(StageAttempt):
    """Implementation of a bootstrap stage."""

    title = 'bootstrap'

    def _operation(self, client):
        with temp_bootstrap_env(get_juju_home(), client):
            client.bootstrap()

    def _result(self, client):
        client.wait_for_started()
        return True


class DestroyEnvironmentAttempt(StageAttempt):
    """Implementation of a destroy-environment stage."""

    title = 'destroy environment'

    def _operation(self, client):
        client.juju('destroy-environment', ('-y', client.env.environment),
                    include_e=False)

    def _result(self, client):
        return True


class EnsureAvailabilityAttempt(StageAttempt):
    """Implementation of an ensure-availability stage."""

    title = 'ensure-availability -n 3'

    def _operation(self, client):
        client.juju('ensure-availability', ('-n', '3'))

    def _result(self, client):
        client.wait_for_ha()
        return True


class DeployManyAttempt(StageAttempt):

    title = 'deploy many'

    def _operation(self, client):
        old_status = client.get_status()
        host_count = 2
        container_count = 2
        for machine in range(host_count):
            client.juju('add-machine', ())
        new_status = client.wait_for_started()
        new_machines = dict(new_status.iter_new_machines(old_status))
        if len(new_machines) != host_count:
            raise AssertionError('Got {} machines, not {}'.format(
                len(new_machines), host_count))
        for machine_name in new_machines:
            target = 'lxc:{}'.format(machine_name)
            for container in range(container_count):
                service = 'ubuntu{}x{}'.format(machine_name, container)
                client.juju('deploy', ('--to', target, 'ubuntu', service))

    def _result(self, client):
        client.wait_for_started()
        return True


def parse_args(args=None):
    """Parse commandline arguments into a Namespace."""
    parser = ArgumentParser()
    parser.add_argument('env')
    parser.add_argument('new_juju_path')
    parser.add_argument('--attempts', type=int, default=2)
    return parser.parse_args(args)


def main():
    args = parse_args()
    stages = [BootstrapAttempt, DeployManyAttempt, EnsureAvailabilityAttempt,
              DestroyEnvironmentAttempt]
    mit = MultiIndustrialTest(args.env, args.new_juju_path,
                              stages, args.attempts)
    results = mit.run_tests()
    sys.stdout.writelines(mit.results_table(results))


if __name__ == '__main__':
    main()
