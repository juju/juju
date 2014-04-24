#!/usr/bin/env python
from __future__ import print_function
__metaclass__ = type


from argparse import ArgumentParser
import os
import random
import re
import string
import subprocess
import sys
import yaml

from jujuconfig import (
    get_environments_path,
    get_jenv_path,
)
from jujupy import (
    check_wordpress,
    Environment,
)
from utility import (
    scoped_environ,
    temp_dir,
    until_timeout,
    wait_for_port,
)


def prepare_environment(environment, already_bootstrapped, machines):
    """Prepare an environment for deployment.

    As well as bootstrapping, this ensures the correct agent version is in
    use.

    :param environment: The name of the environment to use.
    :param already_bootstrapped: If true, the environment is already
        bootstrapped.
    """
    if sys.platform == 'win32':
        # Ensure OpenSSH is never in the path for win tests.
        sys.path = [p for p in sys.path if 'OpenSSH' not in p]
    env = Environment.from_config(environment)
    if not already_bootstrapped:
        env.bootstrap()
    agent_version = env.get_matching_agent_version()
    for ignored in until_timeout(30):
        agent_versions = env.get_status().get_agent_versions()
        if 'unknown' not in agent_versions and len(agent_versions) == 1:
            break
    if agent_versions.keys() != [agent_version]:
        print("Current versions: %s" % ', '.join(agent_versions.keys()))
        env.juju('upgrade-juju', '--version', agent_version)
    env.wait_for_version(env.get_matching_agent_version())
    for machine in machines:
        env.juju('add-machine', machine)
    return env


def destroy_environment(environment):
    if environment.config['type'] != 'manual':
        environment.destroy_environment()
    else:
        destroy_job_instances(os.environ['JOB_NAME'])


def destroy_job_instances(job_name):
    instances = list(get_job_instances(job_name))
    if len(instances) == 0:
        return
    subprocess.check_call(['euca-terminate-instances'] + instances)


def deploy_stack(env, charm_prefix):
    """"Deploy a Wordpress stack in the specified environment.

    :param environment: The name of the desired environment.
    """
    env.deploy(charm_prefix + 'wordpress')
    env.deploy(charm_prefix + 'mysql')
    env.juju('add-relation', 'mysql', 'wordpress')
    env.juju('expose', 'wordpress')
    status = env.wait_for_started().status
    wp_unit_0 = status['services']['wordpress']['units']['wordpress/0']
    check_wordpress(wp_unit_0['public-address'])


def deploy_dummy_stack(env, charm_prefix):
    """"Deploy a dummy stack in the specified environment.
    """
    allowed_chars = string.ascii_uppercase + string.digits
    token=''.join(random.choice(allowed_chars) for n in range(20))
    env.deploy(charm_prefix + 'dummy-source')
    env.juju('set', 'dummy-source', 'token=%s' % token)
    env.deploy(charm_prefix + 'dummy-sink')
    env.juju('add-relation', 'dummy-source', 'dummy-sink')
    env.juju('expose', 'dummy-sink')
    env.wait_for_started()
    # Wait up to 30 seconds for token to be created.
    get_token="""
        for x in $(seq 30); do
          if [ -f /var/run/dummy-sink/token ]; then
            break
          fi
          sleep 1
        done
        cat /var/run/dummy-sink/token
    """
    result = env.client.get_juju_output(env, 'ssh', 'dummy-sink/0', get_token)
    result = re.match(r'([^\n\r]*)\r?\n?', result).group(1)
    if result != token:
        raise ValueError('Token is %r' % result)


def scp_logs(log_names, directory):
    subprocess.check_call(['timeout', '5m', 'scp'] + log_names + [directory])


def dump_logs(env, host, directory):
    log_names = []
    if env.local:
        local = os.path.join(os.environ['JUJU_HOME'], 'local')
        log_names = [os.path.join(local, 'cloud-init-output.log')]
        log_dir = os.path.join(local, 'log')
        log_names.extend(os.path.join(log_dir, l) for l
                         in os.listdir(log_dir) if l.endswith('.log'))
        scp_logs(log_names, directory)
    else:
        log_names = [
            'ubuntu@%s:/var/log/%s' % (host, n)
            for n in ['juju/all-machines.log', 'cloud-init-output.log']]
        wait_for_port(host, 22, timeout=60)
        for log_name in log_names:
            try:
                scp_logs([log_name], directory)
            except subprocess.CalledProcessError:
                pass
    for log_name in os.listdir(directory):
        if not log_name.endswith('.log'):
            continue
        path = os.path.join(directory, log_name)
        subprocess.check_call(['gzip', path])


def test_upgrade(environment):
    env = Environment.from_config(environment)
    upgrade_juju(env)
    env.wait_for_version(env.get_matching_agent_version())


def upgrade_juju(environment):
    environment.set_testing_tools_metadata_url()
    print(
        'The tools-metadata-url is %s' % environment.client.get_env_option(
        environment, 'tools-metadata-url'))
    environment.upgrade_juju()


def get_job_instances(job_name):
    instance_pattern = re.compile('^INSTANCE\t(i-[^\t]*)\t.*')
    description = subprocess.check_output([
        'euca-describe-instances', '--filter', 'instance-state-name=running',
        '--filter', 'tag:job_name=%s' % job_name])
    for line in description.splitlines():
        match = instance_pattern.match(line)
        if match:
            yield match.group(1)


def bootstrap_from_env(juju_home, env):
    new_config = {'environments': {env.environment: env.config}}
    jenv_path = get_jenv_path(juju_home, env.environment)
    with temp_dir() as temp_juju_home:
        if os.path.lexists(jenv_path):
            raise Exception('%s already exists!' % jenv_path)
        new_jenv_path = get_jenv_path(temp_juju_home, env.environment)
        # Create a symlink to allow access while bootstrapping, and to reduce
        # races.  Can't use a hard link because jenv doesn't exist until
        # partway through bootstrap.
        os.symlink(new_jenv_path, jenv_path)
        temp_environments = get_environments_path(temp_juju_home)
        with open(temp_environments, 'w') as config_file:
            yaml.safe_dump(new_config, config_file)
        with scoped_environ():
            os.environ['JUJU_HOME'] = temp_juju_home
            env.bootstrap()
        # replace symlink with file before deleting temp home.
        os.rename(new_jenv_path, jenv_path)


def deploy_job():
    machines = ['ssh:%s' % m for m in os.environ['MACHINES'].split()]
    environment = os.environ['ENV']
    try:
        if sys.platform == 'win32':
            # Ensure OpenSSH is never in the path for win tests.
            sys.path = [p for p in sys.path if 'OpenSSH' not in p]
        env = Environment.from_config(environment)
        host = env.config.get('bootstrap-host')
        try:
            env.bootstrap()
        except:
            if host is not None:
                dump_logs(env, host,
                          os.path.join(os.environ['WORKSPACE'], 'artifacts'))
            raise
        try:
            while True:
                bootstrap = env.get_status().status['machines']['0']
                host = bootstrap.get('dns-name')
                if host is not None and not host.startswith('172.'):
                    break
            try:
                prepare_environment(environment, already_bootstrapped=True,
                                    machines=machines)
                if sys.platform == 'win32':
                    # The win client tests only verify the client to the
                    # state-server.
                    return
                deploy_dummy_stack(env, os.environ['CHARM_PREFIX'])
                if os.environ.get('UPGRADE') == 'true':
                    with scoped_environ():
                        os.environ['PATH'] = os.environ['NEW_PATH']
                        test_upgrade(environment)
            except:
                dump_logs(env, host,
                          os.path.join(os.environ['WORKSPACE'], 'artifacts'))
                raise
        finally:
            destroy_environment(env)
    except Exception as e:
        raise
        print('%s (%s)' % (e, type(e).__name__))
        sys.exit(1)


def main():
    parser = ArgumentParser('Deploy a WordPress stack')
    parser.add_argument('--charm-prefix', help='A prefix for charm urls.',
                        default='')
    parser.add_argument('--already-bootstrapped',
                        help='The environment is already bootstrapped.',
                        action='store_true')
    parser.add_argument('--machine',
                        help='A machine to add to the environment.',
                        action='append', default=[])
    parser.add_argument('--dummy', help='Use dummy charms.',
                        action='store_true')
    parser.add_argument('env', help='The environment to deploy on.')
    args = parser.parse_args()
    try:
        env = prepare_environment(args.env, args.already_bootstrapped,
                                  args.machine)
        if sys.platform == 'win32':
            # The win client tests only verify the client to the state-server.
            return
        if args.dummy:
            deploy_dummy_stack(env, args.charm_prefix)
        else:
            deploy_stack(env, args.charm_prefix)
    except Exception as e:
        print('%s (%s)' % (e, type(e).__name__))
        sys.exit(1)


if __name__ == '__main__':
    main()
