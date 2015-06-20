"""Remote helper class for communicating with juju machines."""

__metaclass__ = type

import abc
import logging
import subprocess

import winrm

from substrate import MAASAccount
import utility


def _remote_for_series(series):
    """Give an appropriate remote class based on machine series."""
    if series is not None and series.startswith("win"):
        return WinRmRemote
    return SSHRemote


def remote_from_unit(client, unit, series=None, status=None):
    """Create remote instance given a juju client and a unit."""
    if series is None:
        if status is None:
            status = client.get_status()
        machine = status.get_unit(unit).get("machine")
        if machine is not None:
            series = status.status["machines"].get(machine, {}).get("series")
    remotecls = _remote_for_series(series)
    return remotecls(client, unit, None, series=series, status=status)


def remote_from_address(address, series=None):
    """Create remote instance given an address"""
    remotecls = _remote_for_series(series)
    return remotecls(None, None, address, series=series)


class _Remote:
    """Remote represents a juju machine to access over the network."""

    __metaclass__ = abc.ABCMeta

    def __init__(self, client, unit, address, series=None, status=None):
        if address is None and (client is None or unit is None):
            raise ValueError("Remote needs either address or client and unit")
        self.client = client
        self.unit = unit
        self.use_juju_ssh = unit is not None
        self.address = address
        self.series = series
        self.status = status

    def __repr__(self):
        params = []
        if self.client is not None:
            params.append("env=" + repr(self.client.env.environment))
        if self.unit is not None:
            params.append("unit=" + repr(self.unit))
        if self.address is not None:
            params.append("addr=" + repr(self.address))
        return "<{} {}>".format(self.__class__.__name__, " ".join(params))

    @abc.abstractmethod
    def cat(self, filename):
        """Get the contents of filename from the remote machine."""

    @abc.abstractmethod
    def copy(self, destination_dir, source_globs):
        """Copy files from the remote machine."""

    def is_windows(self):
        """Returns True if remote machine is running windows."""
        return self.series and self.series.startswith("win")

    def _get_status(self):
        if self.status is None:
            self.status = self.client.get_status()
        return self.status

    def _ensure_address(self):
        if self.address:
            return
        if self.client is None:
            raise ValueError("No address or client supplied")
        status = self._get_status()
        unit = status.get_unit(self.unit)
        self.address = unit['public-address']
        # TODO(gz): Avoid special casing MAAS here and in deploy_stack
        if self.client.env.config['type'] == 'maas':
            config = self.client.env.config
            with MAASAccount.manager_from_config(config) as account:
                allocated_ips = account.get_allocated_ips()
            if self.address in allocated_ips:
                self.address = allocated_ips[self.address]


class SSHRemote(_Remote):
    """Remote represents a juju machine to access over the network."""

    _ssh_opts = [
        "-o", "User ubuntu",
        "-o", "UserKnownHostsFile /dev/null",
        "-o", "StrictHostKeyChecking no",
    ]

    timeout = "5m"

    def run(self, command):
        """Run a command on the remote machine."""
        if self.use_juju_ssh:
            try:
                return self.client.get_juju_output("ssh", self.unit, command)
            except subprocess.CalledProcessError as e:
                logging.warning("juju ssh to %r failed: %s", self.unit, e)
                self.use_juju_ssh = False
            self._ensure_address()
        args = ["ssh"]
        args.extend(self._ssh_opts)
        args.extend([self.address, command])
        return self._run_subprocess(args)

    def copy(self, destination_dir, source_globs):
        """Copy files from the remote machine."""
        self._ensure_address()
        args = ["scp", "-C"]
        args.extend(self._ssh_opts)
        args.extend(["{}:{}".format(self.address, f) for f in source_globs])
        args.append(destination_dir)
        self._run_subprocess(args)

    def cat(self, filename):
        """Get the contents of filename from the remote machine."""
        return self.run("cat " + utility.quote(filename))

    def _run_subprocess(self, command):
        if self.timeout:
            command = ["timeout", self.timeout] + command
        return subprocess.check_output(command)


class _SSLSession(winrm.Session):

    def __init__(self, target, auth, transport="ssl"):
        key, cert = auth
        self.url = self._build_url(target, transport)
        self.protocol = winrm.Protocol(self.url, transport=transport,
                                       cert_key_pem=key, cert_pem=cert)


class WinRmRemote(_Remote):

    def __init__(self, *args, **kwargs):
        super(WinRmRemote, self).__init__(*args, **kwargs)
        self._ensure_address()
        certs = utility.get_winrm_certs()
        self.session = _SSLSession(self.address, certs)

    _escape = staticmethod(subprocess.list2cmdline)

    def run_cmd(self, cmd_list):
        """Run cmd and arguments given as a list returning response object."""
        if isinstance(cmd_list, basestring):
            raise ValueError("run_cmd requires a list not a string")
        # pywinrm does not correctly escape arguments, fix up before passing.
        cmd = self._escape(cmd_list[:1])
        args = [self._escape(cmd_list[1:])]
        return self.session.run_cmd(cmd, args)

    def run_ps(self, script):
        """Run string of powershell returning response object."""
        return self.session.run_ps(script)

    def cat(self, filename):
        """Get the contents of filename from the remote machine."""
        result = self.session.run_cmd("type", [self._escape([filename])])
        if result.status_code:
            logging.warning("winrm cat failed %r", result)
        return result.std_out

    def copy(self, destination_dir, source_globs):
        """Copy files from the remote machine."""
        raise NotImplementedError
