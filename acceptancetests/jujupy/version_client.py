# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2017 Canonical Ltd.
#
# This program is free software: you can redistribute it and/or modify it
# under the terms of the Lesser GNU General Public License version 3, as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful, but WITHOUT
# ANY WARRANTY; without even the implied warranties of MERCHANTABILITY,
# SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR PURPOSE.  See the Lesser
# GNU General Public License for more details.
#
# You should have received a copy of the Lesser GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.


from contextlib import contextmanager
import json
from locale import getpreferredencoding
import logging
import os
import re
import subprocess

import yaml

from jujupy.client import (
    CommandComplete,
    Controller,
    _DEFAULT_BUNDLE_TIMEOUT,
    get_cache_path,
    get_jenv_path,
    get_teardown_timeout,
    JESNotSupported,
    _jes_cmds,
    Juju2Backend,
    JujuData,
    KVM_MACHINE,
    LXC_MACHINE,
    make_safe_config,
    ModelClient,
    NoopCondition,
    SimpleEnvironment,
    Status,
    StatusItem,
    SYSTEM,
    unqualified_model_name,
    UpgradeMongoNotSupported,
    NoActiveModel
    )
from jujupy.utility import (
    ensure_deleted,
    scoped_environ,
    split_address_port,
    )


log = logging.getLogger("jujupy.version_client")


class BootstrapMismatch(Exception):

    def __init__(self, arg_name, arg_val, env_name, env_val):
        super(BootstrapMismatch, self).__init__(
            '--{} {} does not match {}: {}'.format(
                arg_name, arg_val, env_name, env_val))


class IncompatibleConfigClass(Exception):
    """Raised when a client is initialised with the wrong config class."""


class VersionNotTestedError(Exception):

    def __init__(self, version):
        super(VersionNotTestedError, self).__init__(
            'Tests for juju {} are no longer supported.'.format(version))


class Juju1XBackend(Juju2Backend):
    """Backend for Juju 1.x versions.

    Uses -e to specify models ("environments", uses JUJU_HOME to specify home
    directory.
    """

    _model_flag = '-e'

    def shell_environ(self, used_feature_flags, juju_home):
        """Generate a suitable shell environment.

        For 2.0-alpha1 and earlier set only JUJU_HOME and not JUJU_DATA.
        """
        env = super(Juju1XBackend, self).shell_environ(used_feature_flags,
                                                       juju_home)
        env['JUJU_HOME'] = juju_home
        del env['JUJU_DATA']
        return env


class Status1X(Status):

    @property
    def model_name(self):
        return self.status['environment']

    def get_applications(self):
        return self.status.get('services', {})

    def condense_status(self, item_value):
        """Condense the scattered agent-* fields into a status dict."""
        def shift_field(dest_dict, dest_name, src_dict, src_name):
            if src_name in src_dict:
                dest_dict[dest_name] = src_dict[src_name]
        condensed = {}
        shift_field(condensed, 'current', item_value, 'agent-state')
        shift_field(condensed, 'version', item_value, 'agent-version')
        shift_field(condensed, 'message', item_value, 'agent-state-info')
        return condensed

    def iter_status(self):
        SERVICE = 'service-status'
        AGENT = 'agent-status'
        for machine_name, machine_value in self.iter_machines(containers=True):
            yield StatusItem(StatusItem.JUJU, machine_name,
                             self.condense_status(machine_value))
        for app_name, app_value in self.get_applications().items():
            if SERVICE in app_value:
                yield StatusItem(
                    StatusItem.APPLICATION, app_name,
                    {StatusItem.APPLICATION: app_value[SERVICE]})
            unit_iterator = self._iter_units_in_application(app_value)
            for unit_name, unit_value in unit_iterator:
                if StatusItem.WORKLOAD in unit_value:
                    yield StatusItem(StatusItem.WORKLOAD,
                                     unit_name, unit_value)
                if AGENT in unit_value:
                    yield StatusItem(
                        StatusItem.JUJU, unit_name,
                        {StatusItem.JUJU: unit_value[AGENT]})
                else:
                    yield StatusItem(StatusItem.JUJU, unit_name,
                                     self.condense_status(unit_value))


class ModelClient2_1(ModelClient):
    """Client for Juju 2.1"""

    REGION_ENDPOINT_PROMPT = 'Enter the API endpoint url for the region:'
    login_user_command = 'login'


class ModelClient2_0(ModelClient2_1):
    """Client for Juju 2.0"""

    def _acquire_model_client(self, name, owner=None):
        """Get a client for a model with the supplied name.

        If the name matches self, self is used.  Otherwise, a clone is used.

        Note: owner is ignored for all clients before 2.1.
        """
        if name == self.env.environment:
            return self
        else:
            env = self.env.clone(model_name=name)
            return self.clone(env=env)


class ModelClientRC(ModelClient2_0):

    def get_bootstrap_args(
            self, upload_tools, config_filename, bootstrap_series=None,
            credential=None, auto_upgrade=False, metadata_source=None,
            no_gui=False, agent_version=None):
        """Return the bootstrap arguments for the substrate."""
        if self.env.joyent:
            # Only accept kvm packages by requiring >1 cpu core, see lp:1446264
            constraints = 'mem=2G cpu-cores=1'
        else:
            constraints = 'mem=2G'
        cloud_region = self.get_cloud_region(self.env.get_cloud(),
                                             self.env.get_region())
        # Note controller name before cloud_region
        args = ['--constraints', constraints,
                self.env.environment,
                cloud_region,
                '--config', config_filename,
                '--default-model', self.env.environment]
        if upload_tools:
            if agent_version is not None:
                raise ValueError(
                    'agent-version may not be given with upload-tools.')
            args.insert(0, '--upload-tools')
        else:
            if agent_version is None:
                agent_version = self.get_matching_agent_version()
            args.extend(['--agent-version', agent_version])
        if bootstrap_series is not None:
            args.extend(['--bootstrap-series', bootstrap_series])
        if credential is not None:
            args.extend(['--credential', credential])
        if metadata_source is not None:
            args.extend(['--metadata-source', metadata_source])
        if auto_upgrade:
            args.append('--auto-upgrade')
        if self.env.bootstrap_to is not None:
            args.extend(['--to', self.env.bootstrap_to])
        if no_gui:
            args.append('--no-gui')
        return tuple(args)


class EnvJujuClient1X(ModelClientRC):
    """Base for all 1.x client drivers."""

    default_backend = Juju1XBackend

    config_class = SimpleEnvironment

    status_class = Status1X

    # The environments.yaml options that are replaced by bootstrap options.
    # For Juju 1.x, no bootstrap options are used.
    bootstrap_replaces = frozenset()

    destroy_model_command = 'destroy-environment'

    supported_container_types = frozenset([KVM_MACHINE, LXC_MACHINE])

    agent_metadata_url = 'tools-metadata-url'

    _show_status = 'status'

    command_set_destroy_model = 'destroy-environment'

    command_set_all = 'all-changes'

    @classmethod
    def _get_env(cls, env):
        if isinstance(env, JujuData):
            raise IncompatibleConfigClass(
                'JujuData cannot be used with {}'.format(cls.__name__))
        return env

    def get_cache_path(self):
        return get_cache_path(self.env.juju_home, models=False)

    def remove_service(self, service):
        self.juju('destroy-service', (service,))

    def backup(self):
        environ = self._shell_environ()
        # juju-backup does not support the -e flag.
        environ['JUJU_ENV'] = self.env.environment
        try:
            # Mutate os.environ instead of supplying env parameter so Windows
            # can search env['PATH']
            with scoped_environ(environ):
                args = ['juju', 'backup']
                log.info(' '.join(args))
                output = subprocess.check_output(args)
        except subprocess.CalledProcessError as e:
            log.info(e.output)
            raise
        log.info(output)
        backup_file_pattern = re.compile('(juju-backup-[0-9-]+\.(t|tar.)gz)')
        match = backup_file_pattern.search(output)
        if match is None:
            raise Exception("The backup file was not found in output: %s" %
                            output)
        backup_file_name = match.group(1)
        backup_file_path = os.path.abspath(backup_file_name)
        log.info("State-Server backup at %s", backup_file_path)
        return backup_file_path

    def restore_backup(self, backup_file):
        return self.get_juju_output('restore', '--constraints', 'mem=2G',
                                    backup_file)

    def restore_backup_async(self, backup_file):
        return self.juju_async('restore', ('--constraints', 'mem=2G',
                                           backup_file))

    def enable_ha(self):
        self.juju('ensure-availability', ('-n', '3'))

    def list_models(self):
        """List the models registered with the current controller."""
        log.info('The model is environment {}'.format(self.env.environment))

    def list_clouds(self, format='json'):
        """List all the available clouds."""
        return {}

    def show_controller(self, format='json'):
        """Show controller's status."""
        return {}

    def get_models(self):
        """return a models dict with a 'models': [] key-value pair."""
        return {}

    def list_controllers(self):
        """List the controllers."""
        log.info(
            'The controller is environment {}'.format(self.env.environment))

    @staticmethod
    def get_controller_member_status(info_dict):
        return info_dict.get('state-server-member-status')

    def action_fetch(self, id, action=None, timeout="1m"):
        """Fetches the results of the action with the given id.

        Will wait for up to 1 minute for the action results.
        The action name here is just used for an more informational error in
        cases where it's available.
        Returns the yaml output of the fetched action.
        """
        # the command has to be "action fetch" so that the -e <env> args are
        # placed after "fetch", since that's where action requires them to be.
        out = self.get_juju_output("action fetch", id, "--wait", timeout)
        status = yaml.safe_load(out)["status"]
        if status != "completed":
            name = ""
            if action is not None:
                name = " " + action
            raise Exception(
                "timed out waiting for action%s to complete during fetch" %
                name)
        return out

    def action_do(self, unit, action, *args):
        """Performs the given action on the given unit.

        Action params should be given as args in the form foo=bar.
        Returns the id of the queued action.
        """
        args = (unit, action) + args

        # the command has to be "action do" so that the -e <env> args are
        # placed after "do", since that's where action requires them to be.
        output = self.get_juju_output("action do", *args)
        action_id_pattern = re.compile(
            'Action queued with id: ([a-f0-9\-]{36})')
        match = action_id_pattern.search(output)
        if match is None:
            raise Exception("Action id not found in output: %s" %
                            output)
        return match.group(1)

    def run(self, commands, applications):
        responses = self.get_juju_output(
            'run', '--format', 'json', '--service', ','.join(applications),
            *commands)
        return json.loads(responses)

    def list_space(self):
        return yaml.safe_load(self.get_juju_output('space list'))

    def add_space(self, space):
        self.juju('space create', (space),)

    def add_subnet(self, subnet, space):
        self.juju('subnet add', (subnet, space))

    def add_user_perms(self, username, models=None, permissions='read'):
        raise JESNotSupported()

    def grant(self, user_name, permission, model=None):
        raise JESNotSupported()

    def revoke(self, username, models=None, permissions='read'):
        raise JESNotSupported()

    def set_model_constraints(self, constraints):
        constraint_strings = self._dict_as_option_strings(constraints)
        retvar, ct = self.juju('set-constraints', constraint_strings)
        return retvar, CommandComplete(NoopCondition(), ct)

    def set_config(self, service, options):
        option_strings = ['{}={}'.format(*item) for item in options.items()]
        self.juju('set', (service,) + tuple(option_strings))

    def get_config(self, service):
        return yaml.safe_load(self.get_juju_output('get', service))

    def get_model_config(self):
        """Return the value of the environment's configured option."""
        return yaml.safe_load(self.get_juju_output('get-env'))

    def get_env_option(self, option):
        """Return the value of the environment's configured option."""
        return self.get_juju_output(
            'get-env', option).decode(getpreferredencoding())

    def set_env_option(self, option, value):
        """Set the value of the option in the environment."""
        option_value = "%s=%s" % (option, value)
        retvar, ct = self.juju('set-env', (option_value,))
        return retvar, CommandComplete(NoopCondition(), ct)

    def unset_env_option(self, option):
        """Unset the value of the option in the environment."""
        retvar, ct = self.juju('set-env', ('{}='.format(option),))
        return retvar, CommandComplete(NoopCondition(), ct)

    def get_model_defaults(self, model_key, cloud=None, region=None):
        log.info('No model-defaults stored for client (attempted get).')

    def set_model_defaults(self, model_key, value, cloud=None, region=None):
        log.info('No model-defaults stored for client (attempted set).')

    def unset_model_defaults(self, model_key, cloud=None, region=None):
        log.info('No model-defaults stored for client (attempted unset).')

    def _cmd_model(self, include_e, controller):
        if controller:
            return self.get_controller_model_name()
        elif self.env is None or not include_e:
            return None
        else:
            return unqualified_model_name(self.model_name)

    def update_user_name(self):
        return

    def _get_substrate_constraints(self):
        if self.env.joyent:
            # Only accept kvm packages by requiring >1 cpu core, see lp:1446264
            return 'mem=2G cpu-cores=1'
        else:
            return 'mem=2G'

    def get_bootstrap_args(self, upload_tools, bootstrap_series=None,
                           credential=None):
        """Return the bootstrap arguments for the substrate."""
        if credential is not None:
            raise ValueError(
                '--credential is not supported by this juju version.')
        constraints = self._get_substrate_constraints()
        args = ('--constraints', constraints)
        if upload_tools:
            args = ('--upload-tools',) + args
        if bootstrap_series is not None:
            env_val = self.env.get_option('default-series')
            if bootstrap_series != env_val:
                raise BootstrapMismatch(
                    'bootstrap-series', bootstrap_series, 'default-series',
                    env_val)
        return args

    def bootstrap(self, upload_tools=False, bootstrap_series=None):
        """Bootstrap a controller."""
        self._check_bootstrap()
        args = self.get_bootstrap_args(upload_tools, bootstrap_series)
        retvar, ct = self.juju('bootstrap', args)
        ct.actual_completion()

    @contextmanager
    def bootstrap_async(self, upload_tools=False):
        self._check_bootstrap()
        args = self.get_bootstrap_args(upload_tools)
        with self.juju_async('bootstrap', args):
            yield
            log.info('Waiting for bootstrap of {}.'.format(
                self.env.environment))

    def get_jes_command(self):
        raise JESNotSupported()

    def enable_jes(self):
        raise JESNotSupported()

    def upgrade_juju(self, force_version=True):
        args = ()
        if force_version:
            version = self.get_matching_agent_version(no_build=True)
            args += ('--version', version)
        if self.env.local:
            args += ('--upload-tools',)
        self._upgrade_juju(args)

    def make_model_config(self):
        config_dict = make_safe_config(self)
        # Strip unneeded variables.
        return config_dict

    def _add_model(self, model_name, config_file):
        seen_cmd = self.get_jes_command()
        if seen_cmd == SYSTEM:
            controller_option = ('-s', self.env.environment)
        else:
            controller_option = ('-c', self.env.environment)
        self.juju(_jes_cmds[seen_cmd]['create'], controller_option + (
            model_name, '--config', config_file), include_e=False)

    def destroy_model(self):
        """With JES enabled, destroy-environment destroys the model."""
        return self.destroy_environment(force=False)

    def kill_controller(self, check=False):
        """Destroy the environment, with force. Hard kill option.

        :return: Subprocess's exit code."""
        retvar, ct = self.juju(
            'destroy-environment', (self.env.environment, '--force', '-y'),
            check=check, include_e=False, timeout=get_teardown_timeout(self))
        return retvar, CommandComplete(NoopCondition(), ct)

    def destroy_controller(self, all_models=False):
        """Destroy the environment, with force. Soft kill option.

        :param all_models: Ignored.
        :raises: subprocess.CalledProcessError if the operation fails."""
        retvar, ct = self.juju(
            'destroy-environment', (self.env.environment, '-y'),
            include_e=False, timeout=get_teardown_timeout(self))
        return retvar, CommandComplete(NoopCondition(), ct)

    def destroy_environment(self, force=True, delete_jenv=False):
        if force:
            force_arg = ('--force',)
        else:
            force_arg = ()
        exit_status, _ = self.juju(
            'destroy-environment',
            (self.env.environment,) + force_arg + ('-y',),
            check=False, include_e=False,
            timeout=get_teardown_timeout(self))
        if delete_jenv:
            jenv_path = get_jenv_path(self.env.juju_home, self.env.environment)
            ensure_deleted(jenv_path)
        return exit_status

    def _get_models(self):
        """return a list of model dicts."""
        try:
            return yaml.safe_load(self.get_juju_output(
                'environments', '-s', self.env.environment, '--format', 'yaml',
                include_e=False))
        except subprocess.CalledProcessError:
            # This *private* method attempts to use a 1.25 JES feature.
            # The JES design is dead. The private method is not used to
            # directly test juju cli; the failure is not a contract violation.
            log.info('Call to JES juju environments failed, falling back.')
            return []

    def get_model_uuid(self):
        raise JESNotSupported()

    def deploy_bundle(self, bundle, timeout=_DEFAULT_BUNDLE_TIMEOUT):
        """Deploy bundle using deployer for Juju 1.X version."""
        self.deployer(bundle, timeout=timeout)

    def deployer(self, bundle_template, name=None, deploy_delay=10,
                 timeout=3600):
        """Deploy a bundle using deployer."""
        bundle = self.format_bundle(bundle_template)
        args = (
            '--debug',
            '--deploy-delay', str(deploy_delay),
            '--timeout', str(timeout),
            '--config', bundle,
        )
        if name:
            args += (name,)
        self.juju('deployer', args)

    def deploy(self, charm, repository=None, to=None, series=None,
               service=None, force=False, storage=None, constraints=None):
        args = [charm]
        if repository is not None:
            args.extend(['--repository', repository])
        if to is not None:
            args.extend(['--to', to])
        if service is not None:
            args.extend([service])
        if storage is not None:
            args.extend(['--storage', storage])
        if constraints is not None:
            args.extend(['--constraints', constraints])
        retvar, ct = self.juju('deploy', tuple(args))
        return retvar, CommandComplete(NoopCondition(), ct)

    def upgrade_charm(self, service, charm_path=None):
        args = (service,)
        if charm_path is not None:
            repository = os.path.dirname(os.path.dirname(charm_path))
            args = args + ('--repository', repository)
        self.juju('upgrade-charm', args)

    def get_controller_client(self):
        """Return a client for the controller model.  May return self."""
        return self

    def get_controller_model_name(self):
        """Return the name of the 'controller' model.

        Return the name of the 1.x environment."""
        return self.env.environment

    def get_controller_endpoint(self):
        """Return the host and port of the state-server leader."""
        endpoint = self.get_juju_output('api-endpoints')
        return split_address_port(endpoint)

    def upgrade_mongo(self):
        raise UpgradeMongoNotSupported()

    def create_cloned_environment(
            self, cloned_juju_home, controller_name, user_name=None):
        """Create a cloned environment.

        `user_name` is unused in this version of juju.
        """
        user_client = self.clone(env=self.env.clone())
        user_client.env.juju_home = cloned_juju_home
        # New user names the controller.
        user_client.env.controller = Controller(controller_name)
        return user_client

    def add_storage(self, unit, storage_type, amount="1"):
        """Add storage instances to service.

        Only type 'disk' is able to add instances.
        """
        self.juju('storage add', (unit, storage_type + "=" + amount))

    def list_storage(self):
        """Return the storage list."""
        return self.get_juju_output('storage list', '--format', 'json')

    def list_storage_pool(self):
        """Return the list of storage pool."""
        return self.get_juju_output('storage pool list', '--format', 'json')

    def create_storage_pool(self, name, provider, size):
        """Create storage pool."""
        self.juju('storage pool create',
                  (name, provider,
                   'size={}'.format(size)))

    def ssh_keys(self, full=False):
        """Give the ssh keys registered for the current model."""
        args = []
        if full:
            args.append('--full')
        return self.get_juju_output('authorized-keys list', *args)

    def add_ssh_key(self, *keys):
        """Add one or more ssh keys to the current model."""
        return self.get_juju_output('authorized-keys add', *keys,
                                    merge_stderr=True)

    def remove_ssh_key(self, *keys):
        """Remove one or more ssh keys from the current model."""
        return self.get_juju_output('authorized-keys delete', *keys,
                                    merge_stderr=True)

    def import_ssh_key(self, *keys):
        """Import ssh keys from one or more identities to the current model."""
        return self.get_juju_output('authorized-keys import', *keys,
                                    merge_stderr=True)

    def list_disabled_commands(self):
        """List all the commands disabled on the model."""
        raw = self.get_juju_output('block list', '--format', 'yaml')
        return yaml.safe_load(raw)

    def disable_command(self, command_set, message=''):
        """Disable a command-set."""
        retvar, ct = self.juju('block {}'.format(command_set), (message, ))
        return retvar, CommandComplete(NoopCondition(), ct)

    def enable_command(self, args):
        """Enable a command-set."""
        retvar, ct = self.juju('unblock', args)
        return retvar, CommandComplete(NoopCondition(), ct)


class EnvJujuClient22(EnvJujuClient1X):

    used_feature_flags = frozenset(['actions'])

    def __init__(self, *args, **kwargs):
        super(EnvJujuClient22, self).__init__(*args, **kwargs)
        self.feature_flags.add('actions')


class EnvJujuClient25(EnvJujuClient1X):
    """Drives Juju 2.5-series clients."""

    used_feature_flags = frozenset()

    def disable_jes(self):
        self.feature_flags.discard('jes')


class EnvJujuClient24(EnvJujuClient25):
    """Similar to EnvJujuClient25."""

    def add_ssh_machines(self, machines):
        for machine in machines:
            self.juju('add-machine', ('ssh:' + machine,))


def get_client_class(version):
    if version.startswith('1.16'):
        raise VersionNotTestedError(version)
    elif re.match('^1\.22[.-]', version):
        client_class = EnvJujuClient22
    elif re.match('^1\.24[.-]', version):
        client_class = EnvJujuClient24
    elif re.match('^1\.25[.-]', version):
        client_class = EnvJujuClient25
    elif re.match('^1\.26[.-]', version):
        raise VersionNotTestedError(version)
    elif re.match('^1\.', version):
        client_class = EnvJujuClient1X
    elif re.match('^2\.0-(alpha|beta)', version):
        raise VersionNotTestedError(version)
    elif re.match('^2\.0-rc[1-3]', version):
        client_class = ModelClientRC
    elif re.match('^2\.0[.-]', version):
        client_class = ModelClient2_0
    elif re.match('^2\.1[.-]', version):
        client_class = ModelClient2_1
    else:
        client_class = ModelClient
    return client_class


def get_full_path(juju_path):
    """Helper to ensure a full path is used.

    If juju_path is None, ModelClient.get_full_path is used.  Otherwise,
    the supplied path is converted to absolute.
    """
    if juju_path is None:
        return ModelClient.get_full_path()
    else:
        return os.path.abspath(juju_path)


def client_from_config(config, juju_path, debug=False, soft_deadline=None):
    """Create a client from an environment's configuration.

    :param config: Name of the environment to use the config from.
    :param juju_path: Path to juju binary the client should wrap.
    :param debug=False: The debug flag for the client, False by default.
    :param soft_deadline: A datetime representing the deadline by which
        normal operations should complete.  If None, no deadline is
        enforced.
    """
    version = ModelClient.get_version(juju_path)
    client_class = get_client_class(str(version))
    if config is None:
        env = client_class.config_class('', {})
    else:
        env = client_class.config_class.from_config(config)
    full_path = get_full_path(juju_path)
    return client_class(env, version, full_path, debug=debug,
                        soft_deadline=soft_deadline)


def client_for_existing(juju_path, juju_data_dir, debug=False,
                        soft_deadline=None, controller_name=None,
                        model_name=None):
    """Create a client for an existing controller/model.

    :param juju_path: Path to juju binary the client should wrap.
    :param juju_data_dir: Path to the juju data directory referring the the
        controller and model.
    :param debug=False: The debug flag for the client, False by default.
    :param soft_deadline: A datetime representing the deadline by which
        normal operations should complete.  If None, no deadline is
        enforced.
    """
    version = ModelClient.get_version(juju_path)
    client_class = get_client_class(str(version))
    full_path = get_full_path(juju_path)
    backend = client_class.default_backend(full_path, version, set(),
                                           debug=debug,
                                           soft_deadline=soft_deadline)
    if controller_name is None:
        current_controller = backend.get_active_controller(juju_data_dir)
        controller_name = current_controller
    if model_name is None:
        current_model = backend.get_active_model(juju_data_dir)
        model_name = current_model
    config = client_class.config_class.for_existing(
        juju_data_dir, controller_name, model_name)
    return client_class(config, version, full_path, debug=debug,
                        soft_deadline=soft_deadline, _backend=backend)
