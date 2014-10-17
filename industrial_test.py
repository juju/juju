#!/usr/bin/env python
__metaclass__ = type

from argparse import ArgumentParser
import logging

from jujuconfig import get_juju_home
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    temp_bootstrap_env,
    uniquify_local,
    )


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
        except Exception:
            return False


class BootstrapAttempt(StageAttempt):
    """Implementation of a bootstrap stage."""

    def _operation(self, client):
        with temp_bootstrap_env(get_juju_home(), client):
            client.bootstrap()

    def _result(self, client):
        client.wait_for_started()
        return True


class DestroyEnvironmentAttempt(StageAttempt):
    """Implementation of a destroy-environment stage."""

    def _operation(self, client):
        client.juju('destroy-environment', ('-y', client.env.environment),
                    include_e=False)

    def _result(self, client):
        return True


def parse_args(args=None):
    """Parse commandline arguments into a Namespace."""
    parser = ArgumentParser()
    parser.add_argument('env')
    parser.add_argument('new_juju_path')
    return parser.parse_args(args)


def main():
    args = parse_args()
    stages = [BootstrapAttempt(), DestroyEnvironmentAttempt()]
    industrial = IndustrialTest.from_args(args.env, args.new_juju_path, stages)
    results = list(industrial.run_attempt())
    print results


if __name__ == '__main__':
    main()
