#!/usr/bin/env python
from __future__ import print_function


from argparse import ArgumentParser
from contextlib import contextmanager
try:
    from contextlib import nested
except ImportError:
    from contextlib import ExitStack as nested
import errno
import glob
import logging
import os
import random
import re
import string
import subprocess
import sys
import time
import yaml
import shutil

from chaos import background_chaos
from jujucharm import (
    local_charm_path,
)
from jujupy import (
    client_from_config,
    client_for_existing,
    FakeBackend,
    fake_juju_client,
    get_juju_home,
    get_machine_dns_name,
    jes_home_path,
    NoProvider,
    SimpleEnvironment,
    temp_bootstrap_env,
    )
from jujupy.client import (
    get_local_root,
)
from jujupy.configuration import get_jenv_path
from remote import (
    remote_from_address,
    remote_from_unit,
    winrm,
)
from substrate import (
    has_nova_instance,
    LIBVIRT_DOMAIN_RUNNING,
    resolve_remote_dns_names,
    start_libvirt_domain,
    stop_libvirt_domain,
    verify_libvirt_domain,
    make_substrate_manager,
)
from utility import (
    generate_default_clean_dir,
    add_basic_testing_arguments,
    configure_logging,
    ensure_deleted,
    ensure_dir,
    logged_exception,
    LoggedException,
    PortTimeoutError,
    print_now,
    until_timeout,
    wait_for_port,
)


__metaclass__ = type


def deploy_dummy_stack(client, charm_series, use_charmstore=False):
    """"Deploy a dummy stack in the specified environment."""
    # Centos requires specific machine configuration (i.e. network device
    # order).
    if charm_series.startswith("centos") and client.env.maas:
        client.set_model_constraints({'tags': 'MAAS_NIC_1'})
    platform = 'ubuntu'
    if charm_series.startswith('win'):
        platform = 'win'
    elif charm_series.startswith('centos'):
        platform = 'centos'
    if use_charmstore:
        dummy_source = "cs:~juju-qa/dummy-source"
        dummy_sink = "cs:~juju-qa/dummy-sink"
    else:
        dummy_source = local_charm_path(
            charm='dummy-source', juju_ver=client.version, series=charm_series,
            platform=platform)
        dummy_sink = local_charm_path(
            charm='dummy-sink', juju_ver=client.version, series=charm_series,
            platform=platform)
    client.deploy(dummy_source, series=charm_series)
    client.deploy(dummy_sink, series=charm_series)
    client.juju('add-relation', ('dummy-source', 'dummy-sink'))
    client.juju('expose', ('dummy-sink',))
    if client.env.kvm or client.env.maas:
        # A single virtual machine may need up to 30 minutes before
        # "apt-get update" and other initialisation steps are
        # finished; two machines initializing concurrently may
        # need even 40 minutes. In addition Windows image blobs or
        # any system deployment using MAAS requires extra time.
        client.wait_for_started(7200)
    else:
        client.wait_for_started(3600)


def assess_juju_relations(client):
    token = get_random_string()
    client.set_config('dummy-source', {'token': token})
    check_token(client, token)


def get_token_from_status(client):
    """Return the token from the application status message or None."""
    status = client.get_status()
    unit = status.get_unit('dummy-sink/0')
    app_status = unit.get('workload-status')
    if app_status is not None:
        message = app_status.get('message', '')
        parts = message.split()
        if parts:
            return parts[-1]
    return None


token_pattern = re.compile(r'([^\n\r]*)\r?\n?')


def _get_token(remote, token_path="/var/run/dummy-sink/token"):
    """Check for token with remote, handling missing error if needed."""
    try:
        contents = remote.cat(token_path)
    except subprocess.CalledProcessError as e:
        if e.returncode != 1:
            raise
        return ""
    return token_pattern.match(contents).group(1)


def check_token(client, token, timeout=120):
    """Check the token found on dummy-sink/0 or raise ValueError."""
    logging.info('Waiting for applications to reach ready.')
    client.wait_for_workloads()
    logging.info('Retrieving token.')
    remote = remote_from_unit(client, "dummy-sink/0")
    # Update remote with real address if needed.
    resolve_remote_dns_names(client.env, [remote])
    # By this point the workloads should be ready and token will have been
    # sent successfully, but fallback to timeout as previously for now.
    start = time.time()
    while True:
        is_winclient1x = sys.platform == "win32"
        if remote.is_windows() or is_winclient1x:
            result = get_token_from_status(client)
            if not result:
                result = _get_token(remote, "%ProgramData%\\dummy-sink\\token")
        else:
            result = _get_token(remote)
        if result == token:
            logging.info("Token matches expected %r", result)
            return
        if time.time() - start > timeout:
            if remote.use_juju_ssh and _can_run_ssh():
                # 'juju ssh' didn't error, but try raw ssh to verify
                # the result is the same.
                remote.get_address()
                remote.use_juju_ssh = False
                result = _get_token(remote)
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
        report_path = os.path.join(log_directory, 'juju_command_times.yaml')
        with open(report_path, 'w') as timing_file:
            yaml.safe_dump(
                client.get_juju_timings(),
                timing_file)
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
    resolve_remote_dns_names(client.env, machines.values())
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
        log_files.extend(os.path.join(r, f) for f in fs if is_log(f))
    if log_files:
        subprocess.check_call(['gzip', '--best', '-f'] + log_files)


def is_log(file_name):
    """Check to see if the given file name is the name of a log file."""
    return file_name.endswith('.log') or file_name.endswith('syslog')


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
            '/var/log/lxd/juju-*',
            '/var/log/lxd/lxd.log',
            '/var/log/syslog',
            '/var/log/mongodb/mongodb.log',
            '/etc/network/interfaces',
            '/etc/environment',
            '/home/ubuntu/ifconfig.log',
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
            remote.run('ifconfig > /home/ubuntu/ifconfig.log')
        except subprocess.CalledProcessError as e:
            logging.warning("Could not capture ifconfig state:")
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
    responses = client.run(('uname',),
                           applications=['dummy-source', 'dummy-sink'])
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
    all_clients = _get_clients_to_upgrade(old_client, juju_path)

    # all clients have the same provider type, work this out once.
    if all_clients[0].env.provider == 'maas':
        timeout = 1200
    else:
        timeout = 600

    for client in all_clients:
        logging.info('Upgrading {}'.format(client.env.environment))
        upgrade_juju(client)
        client.wait_for_version(client.get_matching_agent_version(), timeout)
        logging.info('Agents upgraded in {}'.format(client.env.environment))
        client.show_status()
        logging.info('Waiting for model {}'.format(client.env.environment))
        # While the agents are upgraded, the controller/model may still be
        # upgrading. We are only certain that the upgrade as is complete
        # when we can list models.
        for ignore in until_timeout(600):
            try:
                client.list_models()
                break
            except subprocess.CalledProcessError:
                pass
        # The upgrade will trigger the charm hooks. We want the charms to
        # return to active state to know they accepted the upgrade.
        client.wait_for_workloads()
        logging.info('Upgraded model {}'.format(client.env.environment))


def _get_clients_to_upgrade(old_client, juju_path):
    """Return a list of cloned clients to upgrade.

    Ensure that the controller (if available) is the first client in the list.
    """
    new_client = old_client.clone_from_path(juju_path)
    all_clients = sorted(
        new_client.iter_model_clients(),
        key=lambda m: m.model_name == 'controller',
        reverse=True)

    return all_clients


def upgrade_juju(client):
    client.set_testing_agent_metadata_url()
    tools_metadata_url = client.get_agent_metadata_url()
    logging.info(
        'The {url_type} is {url}'.format(
            url_type=client.agent_metadata_url,
            url=tools_metadata_url))
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
    parser.add_argument(
        '--controller-host', help=(
            'Host with a controller to use.  If supplied, SSO_EMAIL and'
            ' SSO_PASSWORD environment variables will be used for oauth'
            ' authentication.'))
    parser.add_argument('--use-charmstore', action='store_true',
                        help='Deploy dummy charms from the charmstore.')
    return parser.parse_args(argv)


def deploy_job():
    args = deploy_job_parse_args()
    configure_logging(args.verbose)
    series = args.series
    if not args.logs:
        args.logs = generate_default_clean_dir(args.temp_env_name)
    if series is None:
        series = 'precise'
    charm_series = series
    # Don't need windows or centos state servers.
    if series.startswith("win") or series.startswith("centos"):
        logging.info('Setting default series to trusty for win and centos.')
        series = 'trusty'
    return _deploy_job(args, charm_series, series)


def update_env(env, new_env_name, series=None, bootstrap_host=None,
               agent_url=None, agent_stream=None, region=None):
    # Rename to the new name.
    env.set_model_name(new_env_name)
    new_config = {}
    if series is not None:
        new_config['default-series'] = series
    if bootstrap_host is not None:
        new_config['bootstrap-host'] = bootstrap_host
    if agent_url is not None:
        new_config['tools-metadata-url'] = agent_url
    if agent_stream is not None:
        new_config['agent-stream'] = agent_stream
    env.update_config(new_config)
    if region is not None:
        env.set_region(region)


@contextmanager
def temp_juju_home(client, new_home):
    """Temporarily override the client's home directory."""
    old_home = client.env.juju_home
    client.env.juju_home = new_home
    try:
        yield
    finally:
        client.env.juju_home = old_home


def make_controller_strategy(client, tear_down_client, controller_host):
    if controller_host is None:
        return CreateController(client, tear_down_client)
    else:
        return PublicController(
            controller_host, os.environ['SSO_EMAIL'],
            os.environ['SSO_PASSWORD'], client, tear_down_client)


def error_if_unclean(unclean_resources):
    """List all the resource that were not cleaned up programmatically.

    :param unclean_resources: List of unclean resources
    """
    if unclean_resources:
        logging.critical("Following resource requires manual cleanup")
        for resources in unclean_resources:
            resource = resources.get("resource")
            logging.critical(resource)
            errors = resources.get("errors")
            for (id, reason) in errors:
                reason_string = "\t{}: {}".format(id, reason)
                logging.critical(reason_string)


class CreateController:
    """A Controller strategy where the controller is created.

    Intended for use with BootstrapManager.
    """

    def __init__(self, client, tear_down_client):
        self.client = client
        self.tear_down_client = tear_down_client

    def prepare(self):
        """Prepare client for use by killing the existing controller."""
        self.tear_down_client.kill_controller()

    def create_initial_model(self, upload_tools, series, boot_kwargs):
        """Create the initial model by bootstrapping."""
        self.client.bootstrap(
            upload_tools=upload_tools, bootstrap_series=series,
            **boot_kwargs)

    def get_hosts(self):
        """Provide the controller host."""
        host = get_machine_dns_name(
            self.client.get_controller_client(), '0')
        if host is None:
            raise ValueError('Could not get machine 0 host')
        return {'0': host}

    def tear_down(self, has_controller):
        """Tear down via client.tear_down."""
        if has_controller:
            self.tear_down_client.tear_down()
        else:
            self.tear_down_client.kill_controller(check=True)


class ExistingController:
    """A Controller strategy where the controller is already present.

    Intended for use with BootstrapManager and client.client_for_existing().

    :ivar client: Client object
    :ivar tear_down_client: Client object to tear down at the end of testing
    """

    def __init__(self, client):
        self.client = client
        self.tear_down_client = client

    def create_initial_model(self):
        """Create the initial model for use in testing.

        Since we set client.env.environment to our desired model name jujupy
        picks that up to name the new model.
        """

        self.client.add_model(self.client.env)
        logging.info('Added model {} to existing controller'.format(
            self.client.env.environment))

    def prepare(self, controller_name):
        """Prepare client for use by pointing it at the selected controller.

        This is a bit of a hack to allow for multiple controllers in the same
        environment while testing. When the client object is intiailly made out
        of the existing environment it picks up the current controller and
        sets the env.controller name to that ID. Resetting the name to occurred
        desired ID simply forces jujupy to pass that ID as the first part of
        the -m <controller>:<model> flag for commands.

        :param controller_id: ID of the controller in use for testing, passed
        in with the --existing flag
        """
        self.client.env.controller.name = controller_name

    def get_hosts(self):
        """Provide the controller host."""
        host = get_machine_dns_name(
            self.client.get_controller_client(), '0')
        if host is None:
            raise ValueError('Could not get machine 0 host')
        return {'0': host}

    def tear_down(self, _):
        """Destroys the current model"""
        self.client.destroy_model()


class PublicController:
    """A controller strategy where the controller is public.

    The user registers with the controller, and adds the initial model.
    """
    def __init__(self, controller_host, email, password, client,
                 tear_down_client):
        self.controller_host = controller_host
        self.email = email
        self.password = password
        self.client = client
        self.tear_down_client = tear_down_client

    def prepare(self):
        """Prepare by destroying the model and unregistering if possible."""
        try:
            self.tear_down(True)
        except subprocess.CalledProcessError:
            # Assume that any error tearing down means that there was nothing
            # to tear down.
            pass

    def create_initial_model(self, upload_tools, series, boot_kwargs):
        """Register controller and add model."""
        self.client.register_host(
            self.controller_host, self.email, self.password)
        self.client.env.controller.explicit_region = True
        self.client.add_model(self.client.env)

    def get_hosts(self):
        """There are no user-owned controller hosts, so no-op."""
        return {}

    def tear_down(self, has_controller):
        """Remove the current model and clean up the controller."""
        try:
            self.tear_down_client.destroy_model()
        finally:
            controller = self.tear_down_client.env.controller.name
            self.tear_down_client.juju('unregister', ('-y', controller),
                                       include_e=False)


class BootstrapManager:
    """
    Helper class for running juju tests.

    Enables running tests on the manual provider and on MAAS systems, with
    automatic cleanup, logging, etc.  See BootstrapManager.booted_context.

    :ivar temp_env_name: a unique name for the juju env, such as a Jenkins
        job name.
    :ivar client: a ModelClient.
    :ivar tear_down_client: a ModelClient for tearing down the controller
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
                 keep_env, permanent, jes_enabled, controller_strategy=None,
                 logged_exception_exit=True):
        """Constructor.

        Please see see `BootstrapManager` for argument descriptions.
        """
        self.temp_env_name = temp_env_name
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
        if controller_strategy is None:
            controller_strategy = CreateController(client, tear_down_client)
        self.controller_strategy = controller_strategy
        self.logged_exception_exit = logged_exception_exit
        self.has_controller = False
        self.resource_details = None

    def ensure_cleanup(self):
        """
        Ensure any required cleanup for the current substrate is done.
        returns list of resource cleanup errors.
        """
        if self.resource_details is not None:
            with make_substrate_manager(self.client.env) as substrate:
                if substrate is not None:
                    return substrate.ensure_cleanup(self.resource_details)
                logging.warning(
                    '{} is an unknown provider.'
                    ' Unable to ensure cleanup.'.format(
                        self.client.env.provider))
        return []

    def collect_resource_details(self):
        """
        Collect and store resource information for the bootstrapped instance.
        """
        resource_details = {}
        try:
            controller_uuid = self.client.get_controller_uuid()
            resource_details['controller-uuid'] = controller_uuid
        except Exception:
            logging.debug('Unable to retrieve controller uuid.')

        try:
            members = self.client.get_controller_members()
            resource_details['instances'] = [
                (m.info['instance-id'], m.info['dns-name'])
                for m in members]
        except Exception:
            logging.debug('Unable to retrieve members list.')

        if resource_details:
            self.resource_details = resource_details

    @property
    def client(self):
        return self.controller_strategy.client

    @property
    def tear_down_client(self):
        return self.controller_strategy.tear_down_client

    @classmethod
    def from_args(cls, args):
        if not args.logs:
            args.logs = generate_default_clean_dir(args.temp_env_name)

        # GZ 2016-08-11: Move this logic into client_from_config maybe?
        if args.juju_bin == 'FAKE':
            env = SimpleEnvironment.from_config(args.env)
            client = fake_juju_client(env=env)
        else:
            client = client_from_config(args.env, args.juju_bin,
                                        debug=args.debug,
                                        soft_deadline=args.deadline)
            if args.to is not None:
                client.env.bootstrap_to = args.to
        return cls.from_client(args, client)

    @classmethod
    def from_existing_controller(cls, args):
        try:
            juju_home = os.environ['JUJU_DATA']
        except KeyError:
            home = os.path.expanduser('~')
            if os.path.isdir(os.path.join(home, '.local/share/juju/')):
                juju_home = os.path.join(home, '.local/share/juju/')
            elif os.path.isdir(os.path.join(home, '.juju/')):
                juju_home = os.path.join(home, '.juju/')
            else:
                raise Exception(
                    'No juju data directory found at ~/.local/share/juju/ '
                    'or ~/.juju If your juju data is located somewhere '
                    'else please set the JUJU_DATA env variable to that path.')
        if not args.logs:
            args.logs = generate_default_clean_dir(args.temp_env_name)
        model = args.temp_env_name.replace('-temp-env', '')

        if args.existing == 'current':
            controller = None
        else:
            controller = args.existing
        client = client_for_existing(args.juju_bin, juju_home,
                                     controller_name=controller,
                                     model_name=model)
        client.has_controller = True
        return cls.from_client_existing(args, client)

    @classmethod
    def from_client_existing(cls, args, client):
        jes_enabled = client.is_jes_enabled()
        controller_strategy = ExistingController(client)
        return cls(
            args.temp_env_name, client, client, args.bootstrap_host,
            args.machine, args.series, args.agent_url, args.agent_stream,
            args.region, args.logs, args.keep_env, permanent=jes_enabled,
            jes_enabled=jes_enabled, controller_strategy=controller_strategy)

    @classmethod
    def from_client(cls, args, client):
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
            if self.client.env.provider == 'maas' and self.machines:
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
            if self.client.env.provider == 'maas' and not self.keep_env:
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

    def tear_down(self, try_jes=False):
        """Tear down the client using tear_down_client.

        Attempts to use the soft method destroy_controller, if that fails
        it will use the hard kill_controller.

        :param try_jes: Ignored."""
        if self.tear_down_client.env is not self.client.env:
            raise AssertionError('Tear down client needs same env!')
        self.controller_strategy.tear_down(self.has_controller)
        self.has_controller = False

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
            self.tear_down_client.kill_controller()
            torn_down = True
        else:
            jes_home = jes_home_path(
                self.client.env.juju_home, self.client.env.environment)
            with temp_juju_home(self.client, jes_home):
                cache_path = self.client.get_cache_path()
                if os.path.isfile(cache_path):
                    # An existing .jenv implies JES was used, because when JES
                    # is enabled, cache.yaml is enabled.
                    self.controller_strategy.prepare()
                    torn_down = True
        ensure_deleted(jenv_path)
        with temp_bootstrap_env(self.client.env.juju_home, self.client,
                                permanent=self.permanent, set_home=False):
            with self.handle_bootstrap_exceptions():
                if not torn_down:
                    self.controller_strategy.prepare()
                self.has_controller = True
                yield

    @contextmanager
    def existing_bootstrap_context(self, machines, omit_config=None):
        """ Context for bootstrapping a state server that shares the
        environment with an existing bootstrap environment.

        Using this context makes it possible to boot multiple simultaneous
        environments that share a JUJU_HOME.

        """
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

        with self.handle_bootstrap_exceptions():
            self.has_controller = True
            yield

    @contextmanager
    def handle_bootstrap_exceptions(self):
        """If an exception is raised during bootstrap, handle it.

        Log the exception, re-raise as a LoggedException.
        Copy logs for the bootstrap host
        Tear down.  (self.keep_env is ignored.)
        """
        try:
            # If an exception is raised that indicates an error, log it
            # before tearing down so that the error is closely tied to
            # the failed operation.
            with logged_exception(logging):
                yield
        except:
            # If run from a windows machine may not have ssh to get
            # logs
            with self.client.ignore_soft_deadline():
                with self.tear_down_client.ignore_soft_deadline():
                    if self.bootstrap_host is not None and _can_run_ssh():
                        remote = remote_from_address(self.bootstrap_host,
                                                     series=self.series)
                        copy_remote_logs(remote, self.log_dir)
                        archive_logs(self.log_dir)
                    self.controller_strategy.prepare()
            raise

    @contextmanager
    def runtime_context(self, addable_machines):
        """Context for running non-bootstrap operations.

        If any manual machines need to be added, they will be added before
        control is yielded.
        """
        try:
            with logged_exception(logging):
                if len(self.known_hosts) == 0:
                    self.known_hosts.update(
                        self.controller_strategy.get_hosts())
                if addable_machines is not None:
                    self.client.add_ssh_machines(addable_machines)
                yield
        except:
            if self.has_controller:
                safe_print_status(self.client)
            else:
                logging.info("Client lost controller, not calling status.")
            raise
        else:
            if self.has_controller:
                with self.client.ignore_soft_deadline():
                    self.client.list_controllers()
                    self.client.list_models()
                    for m_client in self.client.iter_model_clients():
                        m_client.show_status()
        finally:
            with self.client.ignore_soft_deadline():
                with self.tear_down_client.ignore_soft_deadline():
                    try:
                        if self.has_controller:
                            self.dump_all_logs()
                    except KeyboardInterrupt:
                        pass
                    if not self.keep_env:
                        if self.has_controller:
                            self.collect_resource_details()
                        self.tear_down(self.jes_enabled)
                        unclean_resources = self.ensure_cleanup()
                        error_if_unclean(unclean_resources)

    # GZ 2016-08-11: Should this method be elsewhere to avoid poking backend?
    def _should_dump(self):
        return not isinstance(self.client._backend, FakeBackend)

    def dump_all_logs(self, patch_dir=None):
        """Dump logs for all models in the bootstrapped controller."""
        # This is accurate because we bootstrapped self.client.  It might not
        # be accurate for a model created by create_environment.
        if not self._should_dump():
            return
        controller_client = self.client.get_controller_client()
        if not self.jes_enabled:
            clients = [self.client]
        else:
            try:
                clients = list(self.client.iter_model_clients())
            except Exception:
                # Even if the controller is unreachable, we may still be able
                # to gather some logs. The controller_client and self.client
                # instances are all we have knowledge of.
                clients = [controller_client]
                if self.client is not controller_client:
                    clients.append(self.client)
        for client in clients:
            with client.ignore_soft_deadline():
                if client.env.environment == controller_client.env.environment:
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
                artifacts_dir = os.path.join(self.log_dir,
                                             client.env.environment)
                try:
                    os.makedirs(artifacts_dir)
                except OSError as e:
                    if e.errno != errno.EEXIST:
                        raise
                dump_env_logs_known_hosts(
                    client, artifacts_dir, runtime_config, known_hosts)

    @contextmanager
    def top_context(self):
        """Context for running all juju operations in."""
        with self.maas_machines() as machines:
            try:
                yield machines
            finally:
                # This is not done in dump_all_logs because it should be
                # done after tear down.
                if self.log_dir is not None:
                    dump_juju_timings(self.client, self.log_dir)

    @contextmanager
    def booted_context(self, upload_tools, **kwargs):
        """Create a temporary environment in a context manager to run tests in.

        Bootstrap a new environment from a temporary config that is suitable
        to run tests in. Logs will be collected from the machines. The
        environment will be destroyed when the test completes or there is an
        unrecoverable error.

        The temporary environment is created by updating a ModelClient's
        config with series, agent_url, agent_stream.

        :param upload_tools: False or True to upload the local agent instead
            of using streams.
        :param **kwargs: All remaining keyword arguments are passed to the
        client's bootstrap.
        """
        try:
            with self.top_context() as machines:
                with self.bootstrap_context(
                        machines, omit_config=self.client.bootstrap_replaces):
                    self.controller_strategy.create_initial_model(
                        upload_tools, self.series, kwargs)
                with self.runtime_context(machines):
                    self.client.list_controllers()
                    self.client.list_models()
                    for m_client in self.client.iter_model_clients():
                        m_client.show_status()
                    yield machines
        except LoggedException:
            if self.logged_exception_exit:
                sys.exit(1)
            raise

    @contextmanager
    def existing_booted_context(self, upload_tools, **kwargs):
        try:
            with self.top_context() as machines:
                # Existing does less things as there is no pre-cleanup needed.
                with self.existing_bootstrap_context(
                        machines, omit_config=self.client.bootstrap_replaces):
                    self.client.bootstrap(
                        upload_tools=upload_tools,
                        bootstrap_series=self.series,
                        **kwargs)
                with self.runtime_context(machines):
                    yield machines
        except LoggedException:
            sys.exit(1)

    @contextmanager
    def existing_context(self, upload_tools, controller_id):
        try:
            with self.top_context() as machines:
                with self.runtime_context(machines):
                    self.has_controller = True
                    if controller_id != 'current':
                        self.controller_strategy.prepare(controller_id)
                    self.controller_strategy.create_initial_model()
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

    The temporary environment is created by updating a ModelClient's config
    with series, agent_url, agent_stream.

    :param temp_env_name: a unique name for the juju env, such as a Jenkins
        job name.
    :param client: an ModelClient.
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


def _deploy_job(args, charm_series, series):
    start_juju_path = None if args.upgrade else args.juju_bin
    if sys.platform == 'win32':
        # Ensure OpenSSH is never in the path for win tests.
        sys.path = [p for p in sys.path if 'OpenSSH' not in p]
    # GZ 2016-01-22: When upgrading, could make sure to tear down with the
    # newer client instead, this will be required for major version upgrades?
    client = client_from_config(args.env, start_juju_path, args.debug,
                                soft_deadline=args.deadline)
    if args.jes and not client.is_jes_enabled():
        client.enable_jes()
    jes_enabled = client.is_jes_enabled()
    controller_strategy = make_controller_strategy(client, client,
                                                   args.controller_host)
    bs_manager = BootstrapManager(
        args.temp_env_name, client, client, args.bootstrap_host, args.machine,
        series, args.agent_url, args.agent_stream, args.region, args.logs,
        args.keep_env, permanent=jes_enabled, jes_enabled=jes_enabled,
        controller_strategy=controller_strategy)
    with bs_manager.booted_context(args.upload_tools):
        if args.with_chaos > 0:
            manager = background_chaos(args.temp_env_name, client,
                                       args.logs, args.with_chaos)
        else:
            # Create a no-op context manager, to avoid duplicate calls of
            # deploy_dummy_stack(), as was the case prior to this revision.
            manager = nested()
        with manager:
            deploy_dummy_stack(client, charm_series, args.use_charmstore)
        assess_juju_relations(client)
        skip_juju_run = (
            (client.version < "2" and sys.platform in ("win32", "darwin")) or
            charm_series.startswith(("centos", "win")))
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
        with client.ignore_soft_deadline():
            for m_client in client.iter_model_clients():
                m_client.show_status()
    except Exception as e:
        logging.exception(e)


def wait_for_state_server_to_shutdown(host, client, instance_id, timeout=60):
    print_now("Waiting for port to close on %s" % host)
    wait_for_port(host, 17070, closed=True, timeout=timeout)
    print_now("Closed.")
    try:
        provider_type = client.env.provider
    except NoProvider:
        provider_type = None
    if provider_type == 'openstack':
        for ignored in until_timeout(300):
            if not has_nova_instance(client.env, instance_id):
                print_now('{} was removed from nova list'.format(instance_id))
                break
        else:
            raise Exception(
                '{} was not deleted:'.format(instance_id))


def test_on_controller(test, args):
    if args.existing:
        bs_manager = BootstrapManager.from_existing_controller(args)
        with bs_manager.existing_context(args.upload_tools,
                                         args.existing):
                test(bs_manager.client)
    else:
        bs_manager = BootstrapManager.from_args(args)
        with bs_manager.booted_context(args.upload_tools):
                test(bs_manager.client)
