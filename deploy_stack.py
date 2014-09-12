#!/usr/bin/env python
from __future__ import print_function
__metaclass__ = type


from argparse import ArgumentParser
import errno
import glob
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
    if environment.config['type'] == 'manual':
        destroy_job_instances(os.environ['JOB_NAME'])
    else:
        environment.destroy_environment()


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


def run_instances(count, job_name):
    environ = dict(os.environ)
    command = [
        'euca-run-instances', '-k', 'id_rsa', '-n', '%d' % count,
        '-t', 'm1.large', '-g', 'manual-juju-test', 'ami-36aa4d5e']
    run_output = subprocess.check_output(command, env=environ).strip()
    machine_ids = dict(parse_euca(run_output)).keys()
    subprocess.call(
        ['euca-create-tags', '--tag', 'job_name=%s' % job_name]
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
    if env.kvm:
        # A single virtual machine may need up to 30 minutes before
        # "apt-get update" and other initialisation steps are
        # finished; two machines initializing concurrently may
        # need even 40 minutes.
        env.wait_for_started(3600)
    else:
        env.wait_for_started()
    # Wait up to 120 seconds for token to be created.
    # Utopic is slower, maybe because the devel series gets more
    # pckage updates.
    logging.info('Retrieving token.')
    get_token="""
        for x in $(seq 120); do
          if [ -f /var/run/dummy-sink/token ]; then
            if [ "$(cat /var/run/dummy-sink/token)" != "" ]; then
              break
            fi
          fi
          sleep 1
        done
        cat /var/run/dummy-sink/token
    """
    try:
        result = env.client.get_juju_output(
            env, 'ssh', 'dummy-sink/0', get_token)
    except subprocess.CalledProcessError as err:
        print("WARNING: juju ssh failed: {}".format(str(err)))
        print("Falling back to ssh.")
        dummy_sink = env.get_status().status['services']['dummy-sink']
        dummy_sink_ip = dummy_sink['units']['dummy-sink/0']['public-address']
        user_at_host = 'ubuntu@{}'.format(dummy_sink_ip)
        result = subprocess.check_output(
            ['ssh', user_at_host,
             '-o', 'UserKnownHostsFile /dev/null',
             '-o', 'StrictHostKeyChecking no',
             'cat /var/run/dummy-sink/token'])
    result = re.match(r'([^\n\r]*)\r?\n?', result).group(1)
    if result != token:
        raise ValueError('Token is %r' % result)


def dump_env_logs(env, bootstrap_host, directory, host_id=None):
    machine_addrs = get_machines_for_logs(env, bootstrap_host)

    for machine_id, addr in machine_addrs.iteritems():
        logging.info("Retrieving logs for machine-%d", machine_id)
        machine_directory = os.path.join(directory, str(machine_id))
        os.mkdir(machine_directory)
        dump_logs(env, addr, machine_directory)

    dump_euca_console(host_id, directory)


def get_machines_for_logs(env, bootstrap_host):
    # Try to get machine details from environment if possible.
    machine_addrs = dict(get_machine_addrs(env))

    # The bootstrap host always overrides the status output if
    # provided.
    if bootstrap_host:
        machine_addrs['0'] = bootstrap_host

    if env.local and machine_addrs:
        # As per above, we only need one machine for the local
        # provider. Use machine-0 if possible.
        machine_id = min(machine_addrs)
        return {machine_id: machine_addrs[machine_id]}

    return machine_addrs


def get_machine_addrs(env):
    try:
        status = env.get_status()
    except Exception as err:
        logging.warning("Failed to retrieve status for dumping logs: %s", err)
        return
    for machine_id, machine in status.iter_machines():
        hostname = machine.get('dns-name')
        if hostname:
            yield machine_id, hostname


def dump_logs(env, host, directory, host_id=None):
    if env.local:
        copy_local_logs(directory)
    else:
        copy_remote_logs(host, directory)
    subprocess.check_call(
        ['gzip', '-f'] +
        glob.glob(os.path.join(directory, '*.log')))

    dump_euca_console(host_id, directory)


def copy_local_logs(directory):
    local = os.path.join(get_juju_home(), 'local')
    log_names = [os.path.join(local, 'cloud-init-output.log')]
    log_names.extend(glob.glob(os.path.join(local, 'log', '*.log')))

    subprocess.check_call(['sudo', 'chmod', 'go+r'] + log_names)
    subprocess.check_call(['cp'] + log_names + [directory])


def copy_remote_logs(host, directory):
    log_names = [
        'juju/*.log',
        'cloud-init*.log',
    ]
    source = 'ubuntu@%s:/var/log/{%s}' % (host, ','.join(log_names))

    try:
        wait_for_port(host, 22, timeout=60)
    except PortTimeoutError:
        logging.warning("Could not dump logs because port 22 was closed.")
        return

    subprocess.check_call([
        'timeout', '5m', 'ssh',
        '-o', 'UserKnownHostsFile /dev/null',
        '-o', 'StrictHostKeyChecking no',
        'ubuntu@' + host,
        'sudo chmod go+r /var/log/juju/*',
    ])

    subprocess.check_call([
        'timeout', '5m', 'scp', '-C',
        '-o', 'UserKnownHostsFile /dev/null',
        '-o', 'StrictHostKeyChecking no',
        source, directory,
    ])


def dump_euca_console(host_id, directory):
    if host_id is None:
        return
    with open(os.path.join(directory, 'console.log'), 'w') as console_file:
        subprocess.Popen(['euca-get-console-output', host_id],
                         stdout=console_file)


def test_upgrade(old_env):
    env = Environment.from_config(old_env.environment)
    env.client.debug = old_env.client.debug
    upgrade_juju(env)
    env.wait_for_version(env.get_matching_agent_version(), 600)


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


def check_free_disk_space(path, required, purpose):
    df_result = subprocess.check_output(["df", "-k", path])
    df_result = df_result.split('\n')[1]
    df_result = re.split(' +', df_result)
    available = int(df_result[3])
    if available < required:
        message = (
            "Warning: Probably not enough disk space available for\n"
            "%(purpose)s in directory %(path)s,\n"
            "mount point %(mount)s\n"
            "required: %(required)skB, available: %(available)skB."
            )
        print(message % {
            'path': path, 'mount': df_result[5], 'required': required,
            'available': available, 'purpose': purpose
            })


def bootstrap_from_env(juju_home, env):
    if env.config['type'] == 'local':
        env.config.setdefault('root-dir', os.path.join(
            juju_home, env.environment))
    new_config = {'environments': {env.environment: env.config}}
    jenv_path = get_jenv_path(juju_home, env.environment)
    with temp_dir(juju_home) as temp_juju_home:
        if os.path.lexists(jenv_path):
            raise Exception('%s already exists!' % jenv_path)
        new_jenv_path = get_jenv_path(temp_juju_home, env.environment)
        if env.local:
            # MongoDB requires a lot of free disk space, and the only
            # visible error message is from "juju bootstrap":
            # "cannot initiate replication set" if disk space is low.
            # What "low" exactly means, is unclear, but 8GB should be
            # enough.
            check_free_disk_space(temp_juju_home, 8000000, "MongoDB files")
            if env.kvm:
                check_free_disk_space(
                    "/var/lib/uvtool/libvirt/images", 2000000,
                    "KVM disk files")
            else:
                check_free_disk_space(
                    "/var/lib/lxc", 2000000, "LXC containers")
        # Create a symlink to allow access while bootstrapping, and to reduce
        # races.  Can't use a hard link because jenv doesn't exist until
        # partway through bootstrap.
        try:
            os.mkdir(os.path.join(juju_home, 'environments'))
        except OSError as e:
            if e.errno != errno.EEXIST:
                raise
        os.symlink(new_jenv_path, jenv_path)
        try:
            temp_environments = get_environments_path(temp_juju_home)
            with open(temp_environments, 'w') as config_file:
                yaml.safe_dump(new_config, config_file)
            with scoped_environ():
                os.environ['JUJU_HOME'] = temp_juju_home
                env.bootstrap()
            # replace symlink with file before deleting temp home.
            os.rename(new_jenv_path, jenv_path)
        except:
            os.unlink(jenv_path)


def deploy_job():
    from argparse import ArgumentParser
    parser = ArgumentParser('deploy_job')
    parser.add_argument('--new-juju-bin', default=False,
                        help='Dirctory containing the new Juju binary.')
    parser.add_argument('env', help='Base Juju environment.')
    parser.add_argument('workspace', help='Workspace directory.')
    parser.add_argument('job_name', help='Name of the Jenkins job.')
    parser.add_argument('--upgrade', action="store_true", default=False,
                        help='Perform an upgrade test.')
    parser.add_argument('--debug', action="store_true", default=False,
                        help='Use --debug juju logging.')
    parser.add_argument('--series', help='Name of the Ubuntu series to use.')
    parser.add_argument('--run-startup', help='Run common-startup.sh.',
                        action='store_true', default=False)
    parser.add_argument('--bootstrap-host',
                        help='The host to use for bootstrap.')
    parser.add_argument('--machine', help='A machine to add.',
                        action='append', default=[])
    args = parser.parse_args()
    if not args.run_startup:
        juju_path = args.new_juju_bin
    else:
        env = dict(os.environ)
        env.update({
            'ENV': args.env,
            'WORKSPACE': args.workspace,
            })
        scripts = os.path.dirname(os.path.abspath(sys.argv[0]))
        subprocess.check_call(
            ['bash', '{}/common-startup.sh'.format(scripts)], env=env)
        bin_path = subprocess.check_output(['find', 'extracted-bin', '-name',
                                            'juju'])
        juju_path = os.path.abspath(os.path.dirname(bin_path))
    if juju_path is None:
        raise Exception('Either --new-juju-bin or --run-startup must be'
                        ' supplied.')

    new_path = '%s:%s' % (juju_path, os.environ['PATH'])
    log_dir = os.path.join(args.workspace, 'artifacts')
    series = args.series
    if series is None:
        series = 'precise'
    charm_prefix = 'local:{}/'.format(series)
    return _deploy_job(args.job_name, args.env, args.upgrade,
                       charm_prefix, new_path, args.bootstrap_host,
                       args.machine, args.series, log_dir, args.debug)


def update_env(env, new_env_name, series=None, bootstrap_host=None):
    # Rename to the new name.
    env.environment = new_env_name
    # Always bootstrap a matching environment.
    env.config['agent-version'] = env.get_matching_agent_version()
    if series is not None:
        env.config['default-series'] = series
    if bootstrap_host is not None:
        env.config['bootstrap-host'] = bootstrap_host


def _deploy_job(job_name, base_env, upgrade, charm_prefix, new_path,
                bootstrap_host, machines, series, log_dir, debug):
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
        if env.config['type'] == 'manual' and bootstrap_host is None:
            instances = run_instances(3, job_name)
            created_machines = True
            bootstrap_host = instances[0][1]
            bootstrap_id = instances[0][0]
            machines.extend(i[1] for i in instances[1:])
        update_env(env, job_name, bootstrap_host, series)
        try:
            host = bootstrap_host
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
    if env.kvm:
        timeout = 1200
    else:
        timeout = 600
    for remaining in until_timeout(timeout):
        bootstrap = env.get_status(
            timeout=timeout).status['machines'][str(machine)]
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
