#!/usr/bin/env python
from __future__ import print_function


from argparse import ArgumentParser
from contextlib import (
    contextmanager,
    nested,
)
import glob
import logging
import os
import random
import re
import string
import subprocess
import sys
import time
import json
import shutil

from chaos import background_chaos
from jujuconfig import (
    get_jenv_path,
    get_juju_home,
    translate_to_env,
)
from jujupy import (
    EnvJujuClient,
    get_local_root,
    get_machine_dns_name,
    jes_home_path,
    SimpleEnvironment,
    tear_down,
    temp_bootstrap_env,
)
from remote import (
    remote_from_address,
    remote_from_unit,
    winrm,
)
from substrate import (
    destroy_job_instances,
    LIBVIRT_DOMAIN_RUNNING,
    resolve_remote_dns_names,
    run_instances,
    start_libvirt_domain,
    stop_libvirt_domain,
    verify_libvirt_domain,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    ensure_deleted,
    ensure_dir,
    LoggedException,
    PortTimeoutError,
    print_now,
    until_timeout,
    wait_for_port,
)


__metaclass__ = type


def destroy_environment(client, instance_tag):
    client.destroy_environment()
    if (client.env.config['type'] == 'manual' and
            'AWS_ACCESS_KEY' in os.environ):
        destroy_job_instances(instance_tag)


def deploy_dummy_stack(client, charm_prefix):
    """"Deploy a dummy stack in the specified environment."""
    # Centos requires specific machine configuration (i.e. network device
    # order).
    if charm_prefix.startswith("local:centos") and client.env.maas:
        client.set_model_constraints({'tags': 'MAAS_NIC_1'})
    client.deploy(charm_prefix + 'dummy-source')
    client.deploy(charm_prefix + 'dummy-sink')
    client.juju('add-relation', ('dummy-source', 'dummy-sink'))
    client.juju('expose', ('dummy-sink',))
    if client.env.kvm or client.env.maas:
        # A single virtual machine may need up to 30 minutes before
        # "apt-get update" and other initialisation steps are
        # finished; two machines initializing concurrently may
        # need even 40 minutes. In addition Windows image blobs or
        # any system deployment using MAAS requires extra time.
        client.wait_for_started(3600)
    else:
        client.wait_for_started()


def assess_juju_relations(client):
    token = get_random_string()
    client.set_config('dummy-source', {'token': token})
    check_token(client, token)


GET_TOKEN_SCRIPT = """
        for x in $(seq 120); do
          if [ -f /var/run/dummy-sink/token ]; then
            if [ "$(cat /var/run/dummy-sink/token)" != "" ]; then
              break
            fi
          fi
          sleep 1
        done
        cat /var/run/dummy-sink/token
        sleep 2
    """


def check_token(client, token, timeout=120):
    # Wait up to 120 seconds for token to be created.
    # Utopic is slower, maybe because the devel series gets more
    # package updates.
    logging.info('Retrieving token.')
    remote = remote_from_unit(client, "dummy-sink/0")
    # Update remote with real address if needed.
    resolve_remote_dns_names(client.env, [remote])
    start = time.time()
    while True:
        if remote.is_windows():
            try:
                result = remote.cat("%ProgramData%\\dummy-sink\\token")
            except winrm.exceptions.WinRMTransportError as e:
                print("Skipping token check because of: {}".format(str(e)))
        else:
            result = remote.run(GET_TOKEN_SCRIPT)
        token_pattern = re.compile(r'([^\n\r]*)\r?\n?')
        result = token_pattern.match(result).group(1)
        if result == token:
            logging.info("Token matches expected %r", result)
            return
        if time.time() - start > timeout:
            if not remote.is_windows() and remote.use_juju_ssh:
                # 'juju ssh' didn't error, but try raw ssh to verify
                # the result is the same.
                remote.get_address()
                remote.use_juju_ssh = False
                result = remote.run(GET_TOKEN_SCRIPT)
                result = token_pattern.match(result).group(1)
                if result == token:
                    logging.info("Token matches expected %r", result)
                    logging.error("juju ssh to unit is broken.")
            raise ValueError('Token is %r' % result)
        logging.info("Found token %r expected %r", result, token)
        time.sleep(5)


def get_random_string():
    allowed_chars = string.ascii_uppercase + string.digits
    return ''.join(random.choice(allowed_chars) for n in range(20))


def _can_run_ssh():
    """Return true if local system can use ssh to access remote machines."""
    # When client is run on a windows machine, we have no local ssh binary.
    return sys.platform != "win32"


def dump_env_logs(client, bootstrap_host, artifacts_dir, runtime_config=None):
    if bootstrap_host is None:
        known_hosts = {}
    else:
        known_hosts = {'0': bootstrap_host}
    dump_env_logs_known_hosts(client, artifacts_dir, runtime_config,
                              known_hosts)


def dump_env_logs_known_hosts(client, artifacts_dir, runtime_config=None,
                              known_hosts=None):
    if known_hosts is None:
        known_hosts = {}
    if client.env.local:
        logging.info("Retrieving logs for local environment")
        copy_local_logs(client.env, artifacts_dir)
    else:
        remote_machines = get_remote_machines(client, known_hosts)

        for machine_id in sorted(remote_machines, key=int):
            remote = remote_machines[machine_id]
            if not _can_run_ssh() and not remote.is_windows():
                logging.info("No ssh, skipping logs for machine-%s using %r",
                             machine_id, remote)
                continue
            logging.info("Retrieving logs for machine-%s using %r", machine_id,
                         remote)
            machine_dir = os.path.join(artifacts_dir,
                                       "machine-%s" % machine_id)
            ensure_dir(machine_dir)
            copy_remote_logs(remote, machine_dir)
    archive_logs(artifacts_dir)
    retain_config(runtime_config, artifacts_dir)


def retain_config(runtime_config, log_directory):
    if not runtime_config:
        return False

    try:
        shutil.copy(runtime_config, log_directory)
        return True
    except IOError:
        print_now("Failed to copy file. Source: %s Destination: %s" %
                  (runtime_config, log_directory))
    return False


def dump_juju_timings(client, log_directory):
    try:
        with open(os.path.join(log_directory, 'juju_command_times.json'),
                  'w') as timing_file:
            json.dump(client.get_juju_timings(), timing_file, indent=2,
                      sort_keys=True)
            timing_file.write('\n')
    except Exception as e:
        print_now("Failed to save timings")
        print_now(str(e))


def get_remote_machines(client, known_hosts):
    """Return a dict of machine_id to remote machines.

    A bootstrap_host address may be provided as a fallback for machine 0 if
    status fails. For some providers such as MAAS the dns-name will be
    resolved to a real ip address using the substrate api.
    """
    # Try to get machine details from environment if possible.
    machines = dict(iter_remote_machines(client))
    # The bootstrap host is added as a fallback in case status failed.
    for machine_id, address in known_hosts.items():
        if machine_id not in machines:
            machines[machine_id] = remote_from_address(address)
    # Update remote machines in place with real addresses if substrate needs.
    resolve_remote_dns_names(client.env, machines.itervalues())
    return machines


def iter_remote_machines(client):
    try:
        status = client.get_status()
    except Exception as err:
        logging.warning("Failed to retrieve status for dumping logs: %s", err)
        return

    for machine_id, machine in status.iter_machines():
        hostname = machine.get('dns-name')
        if hostname:
            remote = remote_from_address(hostname, machine.get('series'))
            yield machine_id, remote


def archive_logs(log_dir):
    """Compress log files in given log_dir using gzip."""
    log_files = []
    for r, ds, fs in os.walk(log_dir):
        log_files.extend(os.path.join(r, f) for f in fs if f.endswith(".log"))
    if log_files:
        subprocess.check_call(['gzip', '--best', '-f'] + log_files)


lxc_template_glob = '/var/lib/juju/containers/juju-*-lxc-template/*.log'


def copy_local_logs(env, directory):
    """Copy logs for all machines in local environment."""
    local = get_local_root(get_juju_home(), env)
    log_names = [os.path.join(local, 'cloud-init-output.log')]
    log_names.extend(glob.glob(os.path.join(local, 'log', '*.log')))
    log_names.extend(glob.glob(lxc_template_glob))
    try:
        subprocess.check_call(['sudo', 'chmod', 'go+r'] + log_names)
        subprocess.check_call(['cp'] + log_names + [directory])
    except subprocess.CalledProcessError as e:
        logging.warning("Could not retrieve local logs: %s", e)


def copy_remote_logs(remote, directory):
    """Copy as many logs from the remote host as possible to the directory."""
    # This list of names must be in the order of creation to ensure they
    # are retrieved.
    if remote.is_windows():
        log_paths = [
            "%ProgramFiles(x86)%\\Cloudbase Solutions\\Cloudbase-Init\\log\\*",
            "C:\\Juju\\log\\juju\\*.log",
        ]
    else:
        log_paths = [
            '/var/log/cloud-init*.log',
            '/var/log/juju/*.log',
            # TODO(gz): Also capture kvm container logs?
            '/var/lib/juju/containers/juju-*-lxc-*/',
        ]

        try:
            wait_for_port(remote.address, 22, timeout=60)
        except PortTimeoutError:
            logging.warning("Could not dump logs because port 22 was closed.")
            return

        try:
            remote.run('sudo chmod -Rf go+r ' + ' '.join(log_paths))
        except subprocess.CalledProcessError as e:
            # The juju log dir is not created until after cloud-init succeeds.
            logging.warning("Could not allow access to the juju logs:")
            logging.warning(e.output)

    try:
        remote.copy(directory, log_paths)
    except (subprocess.CalledProcessError,
            winrm.exceptions.WinRMTransportError) as e:
        # The juju logs will not exist if cloud-init failed.
        logging.warning("Could not retrieve some or all logs:")
        if getattr(e, 'output', None):
            logging.warning(e.output)
        else:
            logging.warning(repr(e))


def assess_juju_run(client):
    responses = client.get_juju_output('run', '--format', 'json', '--service',
                                       'dummy-source,dummy-sink', 'uname')
    responses = json.loads(responses)
    for machine in responses:
        if machine.get('ReturnCode', 0) != 0:
            raise ValueError('juju run on machine %s returned %d: %s' % (
                             machine.get('MachineId'),
                             machine.get('ReturnCode'),
                             machine.get('Stderr')))
    logging.info(
        "juju run succeeded on machines: %r",
        [str(machine.get("MachineId")) for machine in responses])
    return responses


def assess_upgrade(old_client, juju_path):
    client = EnvJujuClient.by_version(old_client.env, juju_path,
                                      old_client.debug)
    upgrade_juju(client)
    if client.env.config['type'] == 'maas':
        timeout = 1200
    else:
        timeout = 600
    client.wait_for_version(client.get_matching_agent_version(), timeout)


def upgrade_juju(client):
    client.set_testing_tools_metadata_url()
    tools_metadata_url = client.get_env_option('tools-metadata-url')
    logging.info('The tools-metadata-url is %s', tools_metadata_url)
    client.upgrade_juju()


def deploy_job_parse_args(argv=None):
    parser = ArgumentParser('deploy_job')
    add_basic_testing_arguments(parser)
    parser.add_argument('--upgrade', action="store_true", default=False,
                        help='Perform an upgrade test.')
    parser.add_argument('--with-chaos', default=0, type=int,
                        help='Deploy and run Chaos Monkey in the background.')
    parser.add_argument('--jes', action='store_true',
                        help='Use JES to control environments.')
    return parser.parse_args(argv)


def deploy_job():
    args = deploy_job_parse_args()
    configure_logging(args.verbose)
    series = args.series
    if series is None:
        series = 'precise'
    charm_prefix = 'local:{}/'.format(series)
    # Don't need windows or centos state servers.
    if series.startswith("win") or series.startswith("centos"):
        logging.info('Setting default series to trusty for win and centos.')
        series = 'trusty'
    return _deploy_job(args, charm_prefix, series)


def update_env(env, new_env_name, series=None, bootstrap_host=None,
               agent_url=None, agent_stream=None, region=None):
    # Rename to the new name.
    env.environment = new_env_name
    env.config['name'] = new_env_name
    if series is not None:
        env.config['default-series'] = series
    if bootstrap_host is not None:
        env.config['bootstrap-host'] = bootstrap_host
    if agent_url is not None:
        env.config['tools-metadata-url'] = agent_url
    if agent_stream is not None:
        env.config['agent-stream'] = agent_stream
    if region is not None:
        env.config['region'] = region


@contextmanager
def temp_juju_home(client, new_home):
    """Temporarily override the client's home directory."""
    old_home = client.env.juju_home
    client.env.juju_home = new_home
    try:
        yield
    finally:
        client.env.juju_home = old_home


class BootstrapManager:
    """
    Helper class for running juju tests.

    Enables running tests on the manual provider and on MAAS systems, with
    automatic cleanup, logging, etc.  See BootstrapManager.booted_context.

    :ivar temp_env_name: a unique name for the juju env, such as a Jenkins
        job name.
    :ivar client: an EnvJujuClient.
    :ivar tear_down_client: an EnvJujuClient for tearing down the environment
        (may be more reliable/capable/compatible than client.)
    :ivar bootstrap_host: None, or the address of a manual or MAAS host to
        bootstrap on.
    :ivar machine: [] or a list of machines to use add to a manual env
        before deploying services.
    :ivar series: None or the default-series for the temp config.
    :ivar agent_url: None or the agent-metadata-url for the temp config.
    :ivar agent_stream: None or the agent-stream for the temp config.
    :ivar log_dir: The path to the directory to store logs.
    :ivar keep_env: False or True to not destroy the environment and keep
        it alive to do an autopsy.
    :ivar upload_tools: False or True to upload the local agent instead of
        using streams.
    :ivar known_hosts: A dict mapping machine_ids to hosts for
        dump_env_logs_known_hosts.
    """

    def __init__(self, temp_env_name, client, tear_down_client, bootstrap_host,
                 machines, series, agent_url, agent_stream, region, log_dir,
                 keep_env, permanent, jes_enabled):
        """Constructor.

        Please see see `BootstrapManager` for argument descriptions.
        """
        self.temp_env_name = temp_env_name
        self.client = client
        self.tear_down_client = tear_down_client
        self.bootstrap_host = bootstrap_host
        self.machines = machines
        self.series = series
        self.agent_url = agent_url
        self.agent_stream = agent_stream
        self.region = region
        self.log_dir = log_dir
        self.keep_env = keep_env
        if jes_enabled and not permanent:
            raise ValueError('Cannot set permanent False if jes_enabled is'
                             ' True.')
        self.permanent = permanent
        self.jes_enabled = jes_enabled
        self.known_hosts = {}
        if bootstrap_host is not None:
            self.known_hosts['0'] = bootstrap_host

    @classmethod
    def from_args(cls, args):
        env = SimpleEnvironment.from_config(args.env)
        client = EnvJujuClient.by_version(env, args.juju_bin, debug=args.debug)
        jes_enabled = client.is_jes_enabled()
        return cls(
            args.temp_env_name, client, client, args.bootstrap_host,
            args.machine, args.series, args.agent_url, args.agent_stream,
            args.region, args.logs, args.keep_env, permanent=jes_enabled,
            jes_enabled=jes_enabled)

    @contextmanager
    def maas_machines(self):
        """Handle starting/stopping MAAS machines."""
        running_domains = dict()
        try:
            if self.client.env.config['type'] == 'maas' and self.machines:
                for machine in self.machines:
                    name, URI = machine.split('@')
                    # Record already running domains, so we can warn that
                    # we're deleting them following the test.
                    if verify_libvirt_domain(URI, name,
                                             LIBVIRT_DOMAIN_RUNNING):
                        running_domains = {machine: True}
                        logging.info("%s is already running" % name)
                    else:
                        running_domains = {machine: False}
                        logging.info("Attempting to start %s at %s"
                                     % (name, URI))
                        status_msg = start_libvirt_domain(URI, name)
                        logging.info("%s" % status_msg)
                # No further handling of machines down the line is required.
                yield []
            else:
                yield self.machines
        finally:
            # Although this isn't MAAS-related, it was in this context in
            # boot_context.
            logging.info(
                'Juju command timings: {}'.format(
                    self.client.get_juju_timings()))
            dump_juju_timings(self.client, self.log_dir)
            if self.client.env.config['type'] == 'maas' and not self.keep_env:
                logging.info("Waiting for destroy-environment to complete")
                time.sleep(90)
                for machine, running in running_domains.items():
                    name, URI = machine.split('@')
                    if running:
                        logging.warning(
                            "%s at %s was running when deploy_job started."
                            " Shutting it down to ensure a clean environment."
                            % (name, URI))
                    logging.info("Attempting to stop %s at %s" % (name, URI))
                    status_msg = stop_libvirt_domain(URI, name)
                    logging.info("%s" % status_msg)

    @contextmanager
    def aws_machines(self):
        """Handle starting/stopping AWS machines.

        Machines are deliberately killed by tag so that any stray machines
        from previous runs will be killed.
        """
        if (
                self.client.env.config['type'] != 'manual' or
                self.bootstrap_host is not None):
            yield []
            return
        try:
            instances = run_instances(3, self.temp_env_name, self.series)
            new_bootstrap_host = instances[0][1]
            self.known_hosts['0'] = new_bootstrap_host
            yield [i[1] for i in instances[1:]]
        finally:
            if self.keep_env:
                return
            destroy_job_instances(self.temp_env_name)

    def tear_down(self, try_jes=False):
        if self.tear_down_client == self.client:
            jes_enabled = self.jes_enabled
        else:
            jes_enabled = self.tear_down_client.is_jes_enabled()
        if self.tear_down_client.env is not self.client.env:
            raise AssertionError('Tear down client needs same env!')
        tear_down(self.tear_down_client, jes_enabled, try_jes=try_jes)

    @contextmanager
    def bootstrap_context(self, machines, omit_config=None):
        """Context for bootstrapping a state server."""
        bootstrap_host = self.known_hosts.get('0')
        kwargs = dict(
            series=self.series, bootstrap_host=bootstrap_host,
            agent_url=self.agent_url, agent_stream=self.agent_stream,
            region=self.region)
        if omit_config is not None:
            for key in omit_config:
                kwargs.pop(key.replace('-', '_'), None)
        update_env(self.client.env, self.temp_env_name, **kwargs)
        ssh_machines = list(machines)
        if bootstrap_host is not None:
            ssh_machines.append(bootstrap_host)
        for machine in ssh_machines:
            logging.info('Waiting for port 22 on %s' % machine)
            wait_for_port(machine, 22, timeout=120)
        jenv_path = get_jenv_path(self.client.env.juju_home,
                                  self.client.env.environment)
        torn_down = False
        if os.path.isfile(jenv_path):
            # An existing .jenv implies JES was not used, because when JES is
            # enabled, cache.yaml is enabled.
            self.tear_down(try_jes=False)
            torn_down = True
        else:
            jes_home = jes_home_path(
                self.client.env.juju_home, self.client.env.environment)
            with temp_juju_home(self.client, jes_home):
                cache_path = self.client.get_cache_path()
                if os.path.isfile(cache_path):
                    # An existing .jenv implies JES was used, because when JES
                    # is enabled, cache.yaml is enabled.
                    self.tear_down(try_jes=True)
                    torn_down = True
        ensure_deleted(jenv_path)
        with temp_bootstrap_env(self.client.env.juju_home, self.client,
                                permanent=self.permanent, set_home=False):
            try:
                try:
                    if not torn_down:
                        self.tear_down(try_jes=True)
                    yield
                # If an exception is raised that indicates an error, log it
                # before tearing down so that the error is closely tied to
                # the failed operation.
                except Exception as e:
                    logging.exception(e)
                    if getattr(e, 'output', None):
                        print_now('\n')
                        print_now(e.output)
                    raise LoggedException(e)
            except:
                # If run from a windows machine may not have ssh to get
                # logs
                if self.bootstrap_host is not None and _can_run_ssh():
                    remote = remote_from_address(self.bootstrap_host,
                                                 series=self.series)
                    copy_remote_logs(remote, self.log_dir)
                    archive_logs(self.log_dir)
                self.tear_down()
                raise

    @contextmanager
    def runtime_context(self, addable_machines):
        """Context for running non-bootstrap operations.

        If any manual machines need to be added, they will be added before
        control is yielded.
        """
        try:
            try:
                if len(self.known_hosts) == 0:
                    host = get_machine_dns_name(self.client.get_admin_client(),
                                                '0')
                    if host is None:
                        raise ValueError('Could not get machine 0 host')
                    self.known_hosts['0'] = host
                if addable_machines is not None:
                    self.client.add_ssh_machines(addable_machines)
                yield
            # avoid logging GeneratorExit
            except GeneratorExit:
                raise
            except BaseException as e:
                logging.exception(e)
                raise LoggedException(e)
        finally:
            safe_print_status(self.client)
            self.dump_all_logs()
            if not self.keep_env:
                self.tear_down(self.jes_enabled)

    def dump_all_logs(self):
        """Dump logs for all models in the bootstrapped controller."""
        # This is accurate because we bootstrapped self.client.  It might not
        # be accurate for a model created by create_environment.
        admin_client = self.client.get_admin_client()
        if not self.jes_enabled:
            clients = [self.client]
        else:
            try:
                clients = list(self.client.iter_model_clients())
            except Exception:
                # Even if the controller is unreachable, we may still be able
                # to gather some logs.  admin_client and self.client are all
                # we have knowledge of.
                clients = [admin_client]
                if self.client is not admin_client:
                    clients.append(self.client)
        for client in clients:
            if client.env.environment == admin_client.env.environment:
                known_hosts = self.known_hosts
                if self.jes_enabled:
                    runtime_config = self.client.get_cache_path()
                else:
                    runtime_config = get_jenv_path(
                        self.client.env.juju_home,
                        self.client.env.environment)
            else:
                known_hosts = {}
                runtime_config = None
            artifacts_dir = os.path.join(self.log_dir, client.env.environment)
            os.mkdir(artifacts_dir)
            dump_env_logs_known_hosts(
                client, artifacts_dir, runtime_config, known_hosts)

    @contextmanager
    def top_context(self):
        """Context for running all juju operations in."""
        with self.maas_machines() as machines:
            with self.aws_machines() as new_machines:
                yield machines + new_machines

    @contextmanager
    def booted_context(self, upload_tools):
        """Create a temporary environment in a context manager to run tests in.

        Bootstrap a new environment from a temporary config that is suitable
        to run tests in. Logs will be collected from the machines. The
        environment will be destroyed when the test completes or there is an
        unrecoverable error.

        The temporary environment is created by updating a EnvJujuClient's
        config with series, agent_url, agent_stream.

        :param upload_tools: False or True to upload the local agent instead
            of using streams.
        """
        try:
            with self.top_context() as machines:
                with self.bootstrap_context(
                        machines, omit_config=self.client.bootstrap_replaces):
                    self.client.bootstrap(
                        upload_tools, bootstrap_series=self.series)
                with self.runtime_context(machines):
                    yield machines
        except LoggedException:
            sys.exit(1)


@contextmanager
def boot_context(temp_env_name, client, bootstrap_host, machines, series,
                 agent_url, agent_stream, log_dir, keep_env, upload_tools,
                 region=None):
    """Create a temporary environment in a context manager to run tests in.

    Bootstrap a new environment from a temporary config that is suitable to
    run tests in. Logs will be collected from the machines. The environment
    will be destroyed when the test completes or there is an unrecoverable
    error.

    The temporary environment is created by updating a EnvJujuClient's config
    with series, agent_url, agent_stream.

    :param temp_env_name: a unique name for the juju env, such as a Jenkins
        job name.
    :param client: an EnvJujuClient.
    :param bootstrap_host: None, or the address of a manual or MAAS host to
        bootstrap on.
    :param machine: [] or a list of machines to use add to a manual env
        before deploying services.  This is mutated to indicate all machines,
        including new instances, that have been manually added.
    :param series: None or the default-series for the temp config.
    :param agent_url: None or the agent-metadata-url for the temp config.
    :param agent_stream: None or the agent-stream for the temp config.
    :param log_dir: The path to the directory to store logs.
    :param keep_env: False or True to not destroy the environment and keep
        it alive to do an autopsy.
    :param upload_tools: False or True to upload the local agent instead of
        using streams.
    """
    jes_enabled = client.is_jes_enabled()
    bs_manager = BootstrapManager(
        temp_env_name, client, client, bootstrap_host, machines, series,
        agent_url, agent_stream, region, log_dir, keep_env,
        permanent=jes_enabled, jes_enabled=jes_enabled)
    with bs_manager.booted_context(upload_tools) as new_machines:
        machines[:] = new_machines
        yield


def _deploy_job(args, charm_prefix, series):
    start_juju_path = None if args.upgrade else args.juju_bin
    if sys.platform == 'win32':
        # Ensure OpenSSH is never in the path for win tests.
        sys.path = [p for p in sys.path if 'OpenSSH' not in p]
    # GZ 2016-01-22: When upgrading, could make sure to tear down with the
    # newer client instead, this will be required for major version upgrades?
    client = EnvJujuClient.by_version(
        SimpleEnvironment.from_config(args.env), start_juju_path, args.debug)
    if args.jes and not client.is_jes_enabled():
        client.enable_jes()
    jes_enabled = client.is_jes_enabled()
    bs_manager = BootstrapManager(
        args.temp_env_name, client, client, args.bootstrap_host, args.machine,
        series, args.agent_url, args.agent_stream, args.region, args.logs,
        args.keep_env, permanent=jes_enabled, jes_enabled=jes_enabled)
    with bs_manager.booted_context(args.upload_tools):
        if sys.platform in ('win32', 'darwin'):
            # The win and osx client tests only verify the client
            # can bootstrap and call the state-server.
            return
        client.show_status()
        if args.with_chaos > 0:
            manager = background_chaos(args.temp_env_name, client,
                                       args.logs, args.with_chaos)
        else:
            # Create a no-op context manager, to avoid duplicate calls of
            # deploy_dummy_stack(), as was the case prior to this revision.
            manager = nested()
        with manager:
            deploy_dummy_stack(client, charm_prefix)
        assess_juju_relations(client)
        skip_juju_run = charm_prefix.startswith(("local:centos", "local:win"))
        if not skip_juju_run:
            assess_juju_run(client)
        if args.upgrade:
            client.show_status()
            assess_upgrade(client, args.juju_bin)
            assess_juju_relations(client)
            if not skip_juju_run:
                assess_juju_run(client)


def safe_print_status(client):
    """Show the output of juju status without raising exceptions."""
    try:
        client.show_status()
    except Exception as e:
        logging.exception(e)


def wait_for_state_server_to_shutdown(host, client, instance_id):
    print_now("Waiting for port to close on %s" % host)
    wait_for_port(host, 17070, closed=True)
    print_now("Closed.")
    provider_type = client.env.config.get('type')
    if provider_type == 'openstack':
        environ = dict(os.environ)
        environ.update(translate_to_env(client.env.config))
        for ignored in until_timeout(300):
            output = subprocess.check_output(['nova', 'list'], env=environ)
            if instance_id not in output:
                print_now('{} was removed from nova list'.format(instance_id))
                break
        else:
            raise Exception(
                '{} was not deleted:\n{}'.format(instance_id, output))
