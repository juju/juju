#!/usr/bin/env python

__metaclass__ = type

from argparse import ArgumentParser
import logging

from deploy_stack import (
    dump_env_logs,
    get_machine_dns_name,
    safe_print_status,
    temp_bootstrap_env,
)
from jujuconfig import (
    get_juju_home
)
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


class QuickstartTest:

    @classmethod
    def from_args(cls, env, tmp_name, juju_path, log_dir, bundle_path,
                  service_count, series=None, agent_url=None,
                  debug_flag=False):
        env_config = SimpleEnvironment.from_config(env)
        env_config.environment = tmp_name
        env_config.config['series'] = series
        env_config.config['agent_url'] = agent_url
        client = EnvJujuClient.by_version(env_config, juju_path,
                                          debug=debug_flag)
        return cls(client, bundle_path, log_dir, service_count)

    def __init__(self, client, bundle_path, log_dir, service_count):
        self.client = client
        self.bundle_path = bundle_path
        self.log_dir = log_dir
        self.service_count = service_count

    def run(self):
        bootstrap_host = None
        try:
            for step in self.iter_steps():
                logging.info('{}'.format(step))
                if not bootstrap_host:
                    bootstrap_host = step.get('bootstrap_host')
        except BaseException as e:
            logging.exception(e)
            if bootstrap_host:
                dump_env_logs(self.client, bootstrap_host, self.log_dir)
        finally:
            safe_print_status(self.client)
            self.client.destroy_environment(delete_jenv=True)

    def iter_steps(self):
        # Start the quickstart job
        step = {'juju-quickstart': 'Returned from quickstart'}
        juju_home = get_juju_home()
        with temp_bootstrap_env(juju_home, self.client):
            self.client.quickstart(self.bundle_path)
        yield step
        # Get the hostname for machine 0
        step['bootstrap_host'] = get_machine_dns_name(self.client, 0)
        yield step
        # Wait for deploy to start
        self.client.wait_for_deploy_started(self.service_count)
        step['deploy_started'] = 'Deploy stated'
        yield step
        # Wait for all agents to stat
        self.client.wait_for_started(3600)
        step['agents_started'] = 'All Agents started'
        yield step


def main():
    parser = add_basic_testing_arguments(ArgumentParser())
    parser.add_argument('bundle_path',
                        help='URL or path to a bundle')
    parser.add_argument('--service-count', type=int, default=2,
                        help='Minimum number of expected services.')
    args = parser.parse_args()
    configure_logging(args.verbose)
    quickstart = QuickstartTest.from_args(args.env, args.temp_env_name,
                                          args.juju_bin, args.logs,
                                          args.bundle_path, args.service_count,
                                          args.series, args.agent_url,
                                          args.debug)
    quickstart.run()

if __name__ == '__main__':
    main()
