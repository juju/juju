#!/usr/bin/env python

__metaclass__ = type

from argparse import ArgumentParser
import logging
import subprocess

from deploy_stack import (
    dump_env_logs,
    get_machine_dns_name,
    safe_print_status,
)
from jujuconfig import get_juju_home
from jujupy import (
    bootstrap_from_env,
    EnvJujuClient,
    SimpleEnvironment,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


class CSStagingTest:

    @classmethod
    def from_args(cls, env, tmp_name, juju_path, log_dir, charm_store_ip,
                  charm=None, series=None, agent_url=None,
                  debug_flag=False):
        env_config = SimpleEnvironment.from_config(env)
        env_config.environment = tmp_name
        env_config.config['series'] = series
        env_config.config['agent_url'] = agent_url
        client = EnvJujuClient.by_version(env_config, juju_path,
                                          debug=debug_flag)
        return cls(client, charm_store_ip, charm, log_dir)

    def __init__(self, client, charm_store_ip, charm, log_dir):
        self.client = client
        self.charm_store_ip = charm_store_ip
        self.charm = charm
        self.log_dir = log_dir
        self.bootstrap_host = None

    def bootstrap(self):
        juju_home = get_juju_home()
        bootstrap_from_env(juju_home, self.client)
        self.bootstrap_host = get_machine_dns_name(self.client, 0)
        if self.bootstrap_host is None:
            raise Exception('Could not get machine 0 host')

    def remote_run(self, machine, cmd):
        try:
            ssh_output = self.client.get_juju_output('ssh', machine, cmd)
            if ssh_output:
                logging.info('{}'.format(ssh_output))
        except subprocess.CalledProcessError as err:
            logging.exception(err)
            raise

    def run(self):
        remote_cmd = (
            '''sudo bash -c "echo '%s store.juju.ubuntu.com' >> /etc/hosts"'''
            % self.charm_store_ip)
        try:
            self.bootstrap()
            self.remote_run('0', remote_cmd)
            self.client.deploy(self.charm)
            self.client.wait_for_started(3600)
        except BaseException as e:
            logging.exception(e)
        finally:
            safe_print_status(self.client)
            if self.bootstrap_host:
                dump_env_logs(self.client, self.bootstrap_host, self.log_dir)
            self.client.destroy_environment(delete_jenv=True)


def main():
    parser = add_basic_testing_arguments(ArgumentParser())
    parser.add_argument('charm_store_ip',
                        help='IP of the charm store to use.')
    parser.add_argument('--charm', action='store', default='mysql-10',
                        help='Charm to deploy.')
    args = parser.parse_args()
    configure_logging(args.verbose)
    csstaging = CSStagingTest.from_args(args.env, args.temp_env_name,
                                        args.juju_bin, args.logs,
                                        args.charm_store_ip,
                                        args.charm,
                                        args.series, args.agent_url,
                                        args.debug)
    csstaging.run()

if __name__ == '__main__':
    main()
