"""Helpers to create and manage local juju charms."""

from contextlib import contextmanager
import logging
import os
import re
import subprocess
import pexpect
import yaml

from utility import (
    ensure_deleted,
    JujuAssertionError,
    )


__metaclass__ = type


log = logging.getLogger("jujucharm")


class Charm:
    """Representation of a juju charm."""

    DEFAULT_MAINTAINER = "juju-qa@lists.canonical.com"
    DEFAULT_SERIES = ("bionic", "xenial", "trusty")
    DEFAULT_DESCRIPTION = "description"

    NAME_REGEX = re.compile('^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*$')

    def __init__(self, name, summary, maintainer=None, series=None,
                 description=None, storage=None, ensure_valid_name=True):
        if ensure_valid_name and Charm.NAME_REGEX.match(name) is None:
            raise JujuAssertionError(
                'Invalid Juju Charm Name, "{}" does not match "{}".'.format(
                    name, Charm.NAME_REGEX.pattern))
        self.metadata = {
            "name": name,
            "summary": summary,
            "maintainer": maintainer or self.DEFAULT_MAINTAINER,
            "series": series or self.DEFAULT_SERIES,
            "description": description or self.DEFAULT_DESCRIPTION
        }
        if storage is not None:
            self.metadata["storage"] = storage
        self._hook_scripts = {}

    def to_dir(self, directory):
        """Serialize charm into a new directory."""
        with open(os.path.join(directory, "metadata.yaml"), "w") as f:
            yaml.safe_dump(self.metadata, f, default_flow_style=False)
        if self._hook_scripts:
            hookdir = os.path.join(directory, "hooks")
            os.mkdir(hookdir)
            for hookname in self._hook_scripts:
                with open(os.path.join(hookdir, hookname), "w") as f:
                    os.fchmod(f.fileno(), 0o755)
                    f.write(self._hook_scripts[hookname])

    def to_repo_dir(self, repo_dir):
        """Serialize charm into a directory for a repository of charms."""
        charm_dir = os.path.join(
            repo_dir, self.default_series, self.metadata["name"])
        os.makedirs(charm_dir)
        self.to_dir(charm_dir)
        return charm_dir

    @property
    def default_series(self):
        series = self.metadata.get("series", self.DEFAULT_SERIES)
        if series and isinstance(series, (tuple, list)):
            return series[0]
        return series

    def add_hook_script(self, name, script):
        self._hook_scripts[name] = script


def local_charm_path(charm, juju_ver, series=None, repository=None,
                     platform='ubuntu'):
    """Create either Juju 1.x or 2.x local charm path."""
    if juju_ver.startswith('1.'):
        if series:
            series = '{}/'.format(series)
        else:
            series = ''
        local_path = 'local:{}{}'.format(series, charm)
        return local_path
    else:
        charm_dir = {
            'ubuntu': 'charms',
            'win': 'charms-win',
            'centos': 'charms-centos'}
        abs_path = charm
        if repository:
            abs_path = os.path.join(repository, charm)
        elif os.environ.get('JUJU_REPOSITORY'):
            repository = os.path.join(
                os.environ['JUJU_REPOSITORY'], charm_dir[platform])
            abs_path = os.path.join(repository, charm)
        return abs_path


class CharmCommand:
    default_api_url = 'https://api.jujucharms.com/charmstore'

    def __init__(self, charm_bin, api_url=None):
        """Simple charm command wrapper."""
        self.charm_bin = charm_bin
        self.api_url = sane_charm_store_api_url(api_url)

    def _get_env(self):
        return {'JUJU_CHARMSTORE': self.api_url}

    @contextmanager
    def logged_in_user(self, user_email, password):
        """Contextmanager that logs in and ensures user logs out."""
        try:
            self.login(user_email, password)
            yield
        finally:
            try:
                self.logout()
            except Exception as e:
                log.error('Failed to logout: {}'.format(str(e)))
                default_juju_data = os.path.join(
                    os.environ['HOME'], '.local', 'share', 'juju')
                juju_data = os.environ.get('JUJU_DATA', default_juju_data)
                token_file = os.path.join(juju_data, 'store-usso-token')
                cookie_file = os.path.join(os.environ['HOME'], '.go-cookies')
                log.debug('Removing {} and {}'.format(token_file, cookie_file))
                ensure_deleted(token_file)
                ensure_deleted(cookie_file)

    def login(self, user_email, password):
        log.debug('Logging {} in.'.format(user_email))
        try:
            command = pexpect.spawn(
                self.charm_bin, ['login'], env=self._get_env())
            command.expect(r'(?i)Login to Ubuntu SSO')
            command.expect(r'(?i)Press return to select.*\.')
            command.expect(r'(?i)E-Mail:')
            command.sendline(user_email)
            command.expect(r'(?i)Password')
            command.sendline(password)
            command.expect(r'(?i)Two-factor auth')
            command.sendline()
            command.expect(pexpect.EOF)
            if command.isalive():
                raise AssertionError(
                    'Failed to log user in to {}'.format(
                        self.api_url))
        except (pexpect.TIMEOUT, pexpect.EOF) as e:
            raise AssertionError(
                'Failed to log user in: {}'.format(e))

    def logout(self):
        log.debug('Logging out.')
        self.run('logout')

    def run(self, sub_command, *arguments):
        try:
            output = subprocess.check_output(
                [self.charm_bin, sub_command] + list(arguments),
                env=self._get_env(),
                stderr=subprocess.STDOUT)
            return output
        except subprocess.CalledProcessError as e:
            log.error(e.output)
            raise


def sane_charm_store_api_url(url):
    """Ensure the store url includes the right parts."""
    if url is None:
        return CharmCommand.default_api_url
    return '{}/charmstore'.format(url)
