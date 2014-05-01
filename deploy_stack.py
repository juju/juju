#!/usr/bin/env python
from __future__ import print_function
__metaclass__ = type


from argparse import ArgumentParser
import errno
import logging
import os
import random
import re
import string
import subprocess
import sys
from time import sleep
import yaml

from jujuconfig import (
    get_environments_path,
    get_jenv_path,
    get_juju_home,
)
from jujupy import (
    Environment,
)
from utility import (
    PortTimeoutError,
    scoped_environ,
    temp_dir,
    until_timeout,
    wait_for_port,
)


def prepare_environment(env, already_bootstrapped, machines):
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
        env.juju('add-machine', 'ssh:' + machine)
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


def parse_euca(euca_output):
    for line in euca_output.splitlines():
        fields = line.split('\t')
        if fields[0] != 'INSTANCE':
            continue
        yield fields[1], fields[3]


def run_instances(count):
    environ = dict(os.environ)
    command = [
        'euca-run-instances', '-k', 'id_rsa', '-n', '%d' % count,
        '-t', 'm1.large', '-g', 'manual-juju-test', 'ami-36aa4d5e']
    run_output = subprocess.check_output(command, env=environ).strip()
    machine_ids = dict(parse_euca(run_output)).keys()
    subprocess.call(
        ['euca-create-tags', '--tag', 'job_name=%s' % os.environ['JOB_NAME']]
        + machine_ids, env=environ)
    for remaining in until_timeout(300):
        names = dict(describe_instances(machine_ids, env=environ))
        if '' not in names.values():
            return names.items()
        sleep(1)


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
    logging.info('Retrieving token.')
    get_token="""
        for x in $(seq 30); do
          if [ -f /var/run/dummy-sink/token ]; then
            if [ "$(cat /var/run/dummy-sink/token)" != "" ]; then
              break
            fi
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
    subprocess.check_call([
        'timeout', '5m', 'scp', '-o', 'UserKnownHostsFile /dev/null',
        '-o', 'StrictHostKeyChecking no'] + log_names + [directory])


def dump_logs(env, host, directory, host_id=None):
    log_names = []
    if env.local:
        local = os.path.join(get_juju_home(), 'local')
        log_names = [os.path.join(local, 'cloud-init-output.log')]
        log_dir = os.path.join(local, 'log')
        log_names.extend(os.path.join(log_dir, l) for l
                         in os.listdir(log_dir) if l.endswith('.log'))
        scp_logs(log_names, directory)
    else:
        log_names = [
            'ubuntu@%s:/var/log/%s' % (host, n)
            for n in ['juju/all-machines.log', 'cloud-init-output.log']]
        try:
            wait_for_port(host, 22, timeout=60)
        except PortTimeoutError:
            logging.warning("Could not dump logs because port 22 was closed.")
        for log_name in log_names:
            try:
                scp_logs([log_name], directory)
            except subprocess.CalledProcessError:
                pass
    if host_id is not None:
        with open(os.path.join(directory, 'console.log'), 'w') as console_file:
            subprocess.Popen(['euca-get-console-output', host_id],
                             stdout=console_file)
    for log_name in os.listdir(directory):
        if not log_name.endswith('.log'):
            continue
        path = os.path.join(directory, log_name)
        subprocess.check_call(['gzip', path])


def test_upgrade(old_env):
    env = Environment.from_config(old_env.environment)
    env.client.debug = old_env.client.debug
    upgrade_juju(env)
    env.wait_for_version(env.get_matching_agent_version())


def upgrade_juju(environment):
    environment.set_testing_tools_metadata_url()
    print(
        'The tools-metadata-url is %s' % environment.client.get_env_option(
        environment, 'tools-metadata-url'))
    environment.upgrade_juju()


def describe_instances(instances=None, running=False, job_name=None,
                       env=None):
    command = ['euca-describe-instances']
    if job_name is not None:
        command.extend(['--filter', 'tag:job_name=%s' % job_name])
    if running:
        command.extend(['--filter', 'instance-state-name=running'])
    if instances is not None:
        command.extend(instances)
    logging.info(' '.join(command))
    return parse_euca(subprocess.check_output(command, env=env))


def get_job_instances(job_name):
    description = describe_instances(job_name=job_name, running=True)
    return (machine_id for machine_id, name in description)


def bootstrap_from_env(juju_home, env):
    if env.config['type'] == 'local':
        env.config.setdefault('root-dir', os.path.join(juju_home, 'local'))
    new_config = {'environments': {env.environment: env.config}}
    jenv_path = get_jenv_path(juju_home, env.environment)
    with temp_dir(juju_home) as temp_juju_home:
        if os.path.lexists(jenv_path):
            raise Exception('%s already exists!' % jenv_path)
        new_jenv_path = get_jenv_path(temp_juju_home, env.environment)
        # Create a symlink to allow access while bootstrapping, and to reduce
        # races.  Can't use a hard link because jenv doesn't exist until
        # partway through bootstrap.
        try:
            os.mkdir(os.path.join(juju_home, 'environments'))
        except OSError as e:
            if e.errno != errno.EEXIST:
                raise
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
    required = set(['NEW_JUJU_BIN', 'WORKSPACE', 'NEW_JUJU_BIN', 'ENV',
                    'JOB_NAME', 'CHARM_PREFIX'])
    missing = required.difference(os.environ.keys())
    if len(missing) > 0:
        raise Exception('Missing environment variables: %s' %
                        ', '.join(sorted(missing)))
    base_env = os.environ['ENV']
    job_name = os.environ['JOB_NAME']
    charm_prefix = os.environ['CHARM_PREFIX']
    machines = os.environ.get('MACHINES', '').split()
    new_path = '%s:%s' % (os.environ['NEW_JUJU_BIN'], os.environ['PATH'])
    upgrade = bool(os.environ.get('UPGRADE') == 'true')
    debug = bool(os.environ.get('DEBUG') == 'true')
    log_dir = os.path.join(os.environ['WORKSPACE'], 'artifacts')
    bootstrap_host = os.environ.get('BOOTSTRAP_HOST')
    return _deploy_job(job_name, base_env, upgrade, charm_prefix, new_path,
                       bootstrap_host, machines, log_dir, debug)

def _deploy_job(job_name, base_env, upgrade, charm_prefix, new_path,
                bootstrap_host, machines, log_dir, debug):
    logging.basicConfig(
        level=logging.INFO, format='%(asctime)s %(levelname)s %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S')
    bootstrap_id = None
    created_machines = False
    if not upgrade:
        os.environ['PATH'] = new_path
    try:
        if sys.platform == 'win32':
            # Ensure OpenSSH is never in the path for win tests.
            sys.path = [p for p in sys.path if 'OpenSSH' not in p]
        env = Environment.from_config(base_env)
        env.client.debug = debug
        # Rename to the job name.
        env.environment = job_name
        if bootstrap_host is not None:
            env.config['bootstrap-host'] = bootstrap_host
        elif env.config['type'] == 'manual':
            instances = run_instances(3)
            created_machines = True
            env.config['bootstrap-host'] = instances[0][1]
            bootstrap_id = instances[0][0]
            machines.extend(i[1] for i in instances[1:])
        try:
            host = env.config.get('bootstrap-host')
            env.config['agent-version'] = env.get_matching_agent_version()
            ssh_machines = [] + machines
            if host is not None:
                ssh_machines.append(host)
            for machine in ssh_machines:
                logging.info('Waiting for port 22 on %s' % machine)
                wait_for_port(machine, 22, timeout=120)
            juju_home = get_juju_home()
            try:
                os.unlink(get_jenv_path(juju_home, env.environment))
            except OSError as e:
                if e.errno != errno.ENOENT:
                    raise
            try:
                bootstrap_from_env(juju_home, env)
            except:
                if host is not None:
                    dump_logs(env, host, log_dir, bootstrap_id)
                raise
            try:
                if host is None:
                    host = get_machine_dns_name(env, 0)
                if host is None:
                    raise Exception('Could not get machine 0 host')
                try:
                    prepare_environment(
                        env, already_bootstrapped=True, machines=machines)
                    if sys.platform == 'win32':
                        # The win client tests only verify the client to the
                        # state-server.
                        return
                    deploy_dummy_stack(env, charm_prefix)
                    if upgrade:
                        with scoped_environ():
                            os.environ['PATH'] = new_path
                            test_upgrade(env)
                except:
                    if host is not None:
                        dump_logs(env, host, log_dir, bootstrap_id)
                    raise
            finally:
                env.destroy_environment()
        finally:
            if created_machines:
                destroy_job_instances(job_name)
    except Exception as e:
        raise
        print('%s (%s)' % (e, type(e).__name__))
        sys.exit(1)


def get_machine_dns_name(env, machine):
    for remaining in until_timeout(300):
        bootstrap = env.get_status().status['machines'][str(machine)]
        host = bootstrap.get('dns-name')
        if host is not None and not host.startswith('172.'):
            return host


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
        env = Environment.from_config(args.env)
        prepare_environment(env, args.already_bootstrapped, args.machine)
        if sys.platform == 'win32':
            # The win client tests only verify the client to the state-server.
            return
        deploy_dummy_stack(env, args.charm_prefix)
    except Exception as e:
        print('%s (%s)' % (e, type(e).__name__))
        sys.exit(1)


if __name__ == '__main__':
    main()
