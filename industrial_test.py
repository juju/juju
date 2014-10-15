#!/usr/bin/env python
__metaclass__ = type

from argparse import ArgumentParser

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )


class IndustrialTest:

    @classmethod
    def from_args(cls, env, new_juju_path, stage_attempts):
        env = SimpleEnvironment.from_config(env)
        old_client = EnvJujuClient.by_version(env)
        new_client = EnvJujuClient.by_version(env, new_juju_path)
        return cls(old_client, new_client, stage_attempts)

    def __init__(self, old_client, new_client, stage_attempts):
        self.old_client = old_client
        self.new_client = new_client
        self.stage_attempts = stage_attempts

    def run_attempt(self):
        for attempt in self.stage_attempts:
            result = attempt.do_stage()
            yield result
            if False in result:
                try:
                    self.old_client.destroy_environment()
                finally:
                    self.new_client.destroy_environment()
                break


class StageAttempt:

    def do_stage(self, old, new):
        self.do_operation(old)
        self.do_operation(new)
        old_result = self.get_result(old)
        new_result = self.get_result(new)
        return old_result, new_result


class BootstrapAttempt:

    def do_operation(self, client):
        client.bootstrap()

    def get_result(self, client):
        try:
            client.wait_for_started()
        except Exception:
            return False
        else:
            return True


def parse_args(args=None):
    parser = ArgumentParser()
    parser.add_argument('env')
    parser.add_argument('new_juju_path')
    return parser.parse_args(args)


def main():
    args = parse_args()
    industrial = IndustrialTest.from_args(args.env, args.new_juju_path, [])
    industrial.run_attempt()


if __name__ == '__main__':
    main()
