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
    )


def uniquify_local(env):
    if not env.local:
        return
    port_defaults = {
        'api-port': 17070,
        'state-port': 37017,
        'storage-port': 8040,
        'syslog-port': 6514,
    }
    for key, default in port_defaults.items():
        env.config[key] = env.config.get(key, default) + 1


class IndustrialTest:

    @classmethod
    def from_args(cls, env, new_juju_path, stage_attempts):
        old_env = SimpleEnvironment.from_config(env)
        old_env.environment = env + '-old'
        old_client = EnvJujuClient.by_version(old_env)
        new_env = SimpleEnvironment.from_config(env)
        new_env.environment = env + '-new'
        uniquify_local(new_env)
        new_client = EnvJujuClient.by_version(new_env, new_juju_path)
        return cls(old_client, new_client, stage_attempts)

    def __init__(self, old_client, new_client, stage_attempts):
        self.old_client = old_client
        self.new_client = new_client
        self.stage_attempts = stage_attempts

    def run_attempt(self):
        self.old_client.destroy_environment()
        self.new_client.destroy_environment()
        return self.run_stages()

    def run_stages(self):
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

    def __init__(self):
        self.failure_clients = set()

    def do_stage(self, old, new):
        self.do_operation(old)
        self.do_operation(new)
        old_result = self.get_result(old)
        new_result = self.get_result(new)
        return old_result, new_result

    def do_operation(self, client, output=None):
        try:
            self._operation(client)
        except Exception as e:
            logging.exception(e)
            self.failure_clients.add(client)

    def get_result(self, client):
        if client in self.failure_clients:
            return False
        try:
            return self._result(client)
        except Exception:
            return False


class BootstrapAttempt(StageAttempt):

    def _operation(self, client):
        with temp_bootstrap_env(get_juju_home(), client):
            client.bootstrap()

    def _result(self, client):
        client.wait_for_started()
        return True


class DestroyEnvironmentAttempt(StageAttempt):

    def _operation(self, client):
        client.juju('destroy-environment', ('-y', client.env.environment),
                    include_e=False)

    def _result(self, client):
        return True



def parse_args(args=None):
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
