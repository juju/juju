"""Remote helper class for communicating with juju machines."""
import abc
import logging
import os
import subprocess
import sys
import zlib

import winrm

import utility
import jujupy


__metaclass__ = type


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
    """_Remote represents a juju machine to access over the network."""

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
        """
        Get the contents of filename from the remote machine.

        Environment variables in the filename will be expanded in a according
        to platform-specific rules.
        """

    @abc.abstractmethod
    def copy(self, destination_dir, source_globs):
        """Copy files from the remote machine."""

    def is_windows(self):
        """Returns True if remote machine is running windows."""
        return self.series and self.series.startswith("win")

    def get_address(self):
        """Gives the address of the remote machine."""
        self._ensure_address()
        return self.address

    def update_address(self, address):
        """Change address of remote machine."""
        self.address = address

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
        if 'public-address' not in unit:
            raise ValueError("No public address for unit: {!r} {!r}".format(
                self.unit, unit))
        self.address = unit['public-address']


def _default_is_command_error(err):
    """
    Whether to treat error as issue with remote command rather than ssh.

    This is a conservative default, remote commands may return a variety of
    other return codes. However, as the fallback to local ssh binary will
    repeat the command, those problems will be exposed later anyway.
    """
    return err.returncode == 1


def _no_platform_ssh():
    """True if no openssh binary is available on this platform."""
    return sys.platform == "win32"


class SSHRemote(_Remote):
    """SSHRemote represents a juju machine to access using ssh."""

    _ssh_opts = [
        "-o", "User ubuntu",
        "-o", "UserKnownHostsFile /dev/null",
        "-o", "StrictHostKeyChecking no",
        "-o", "PasswordAuthentication no",
        # prevent "permanently added to known hosts" warning from polluting
        # test log output
        "-o", "LogLevel ERROR",
    ]

    # Limit each operation over SSH to 2 minutes by default
    timeout = 120

    def run(self, command_args, is_command_error=_default_is_command_error):
        """
        Run a command on the remote machine.

        If the remote instance has a juju unit run will default to using the
        juju ssh command. Otherwise, or if that fails, it will fall back to
        using ssh directly.

        The command_args param is a string or list of arguments to be invoked
        on the remote machine. A string must be given if special shell
        characters are used.

        The is_command_error param is a function that takes an instance of
        CalledProcessError and returns whether that error comes from the
        command being run rather than ssh itself. This can be used to skip the
        fallback to native ssh behaviour when running commands that may fail.
        """
        if not isinstance(command_args, (list, tuple)):
            command_args = [command_args]
        if self.use_juju_ssh:
            logging.debug('juju ssh {}'.format(self.unit))
            try:
                return self.client.get_juju_output(
                    "ssh", self.unit, *command_args, timeout=self.timeout)
            except subprocess.CalledProcessError as e:
                logging.warning(
                    "juju ssh to {!r} failed, returncode: {} output: {!r}"
                    " stderr: {!r}".format(
                        self.unit, e.returncode, e.output,
                        getattr(e, "stderr", None)))
                # Don't fallback to calling ssh directly if command really
                # failed or if there is likely to be no usable ssh client.
                if is_command_error(e) or _no_platform_ssh():
                    raise
                self.use_juju_ssh = False
            self._ensure_address()
        args = ["ssh"]
        args.extend(self._ssh_opts)
        args.append(self.address)
        args.extend(command_args)
        logging.debug(' '.join(utility.quote(i) for i in args))
        return self._run_subprocess(args).decode('utf-8')

    def copy(self, destination_dir, source_globs):
        """Copy files from the remote machine."""
        self._ensure_address()
        args = ["scp", "-rC"]
        args.extend(self._ssh_opts)
        address = utility.as_literal_address(self.address)
        args.extend(["{}:{}".format(address, f) for f in source_globs])
        args.append(destination_dir)
        self._run_subprocess(args)

    def cat(self, filename):
        """
        Get the contents of filename from the remote machine.

        Tildes and environment variables in the form $TMP will be expanded.
        """
        return self.run(["cat", filename])

    def use_ssh_key(self, identity_file):
        if "-i" in self._ssh_opts:
            return
        self._ssh_opts.append("-i")
        self._ssh_opts.append(identity_file)

    def _run_subprocess(self, command):
        if self.timeout:
            command = jujupy.get_timeout_prefix(self.timeout) + tuple(command)
        return subprocess.check_output(command, stdin=subprocess.PIPE)


class _SSLSession(winrm.Session):

    def __init__(self, target, auth, transport="ssl"):
        key, cert = auth
        self.url = self._build_url(target, transport)
        self.protocol = winrm.Protocol(self.url, transport=transport,
                                       cert_key_pem=key, cert_pem=cert)


_ps_copy_script = """\
$ErrorActionPreference = "Stop"

function OutputEncodedFile {
    param([String]$filename, [IO.Stream]$instream)
    $trans = New-Object Security.Cryptography.ToBase64Transform
    $out = [Console]::OpenStandardOutput()
    $bs = New-Object Security.Cryptography.CryptoStream($out, $trans,
        [Security.Cryptography.CryptoStreamMode]::Write)
    $zs = New-Object IO.Compression.DeflateStream($bs,
        [IO.Compression.CompressionMode]::Compress)
    [Console]::Out.Write($filename + "|")
    try {
        $instream.CopyTo($zs)
    } finally {
        $zs.close()
        $bs.close()
        [Console]::Out.Write("`n")
    }
}

function GatherFiles {
    param([String[]]$patterns)
    ForEach ($pattern in $patterns) {
        $path = [Environment]::ExpandEnvironmentVariables($pattern)
        ForEach ($file in Get-Item -path $path) {
            try {
                $in = New-Object IO.FileStream($file, [IO.FileMode]::Open,
                    [IO.FileAccess]::Read, [IO.FileShare]"ReadWrite,Delete")
                OutputEncodedFile -filename $file.name -instream $in
            } catch {
                $utf8 = New-Object Text.UTF8Encoding($False)
                $errstream = New-Object IO.MemoryStream(
                    $utf8.GetBytes($_.Exception), $False)
                $errfilename = $file.name + ".copyerror"
                OutputEncodedFile -filename $errfilename -instream $errstream
            }
        }
    }
}

try {
    GatherFiles -patterns @(%s)
} catch {
    Write-Error $_.Exception
    exit 1
}
"""


class WinRmRemote(_Remote):
    """WinRmRemote represents a juju machine to access using winrm."""

    def __init__(self, *args, **kwargs):
        super(WinRmRemote, self).__init__(*args, **kwargs)
        self._ensure_address()
        self.use_juju_ssh = False
        self.certs = utility.get_winrm_certs()
        self.session = _SSLSession(self.address, self.certs)

    def update_address(self, address):
        """Change address of remote machine, refreshes the winrm session."""
        self.address = address
        self.session = _SSLSession(self.address, self.certs)

    _escape = staticmethod(subprocess.list2cmdline)

    def run_cmd(self, cmd_list):
        """Run cmd and arguments given as a list returning response object."""
        if isinstance(cmd_list, (str, bytes)):
            raise ValueError("run_cmd requires a list not a string")
        # pywinrm does not correctly escape arguments, fix up by escaping cmd
        # and giving args as a list of a single pre-escaped string.
        cmd = self._escape(cmd_list[:1])
        args = [self._escape(cmd_list[1:])]
        return self.session.run_cmd(cmd, args)

    def run_ps(self, script):
        """Run string of powershell returning response object."""
        return self.session.run_ps(script)

    def cat(self, filename):
        """
        Get the contents of filename from the remote machine.

        Backslashes will be treated as directory seperators. Environment
        variables in the form %TMP% will be expanded.
        """
        result = self.session.run_cmd("type", [self._escape([filename])])
        if result.status_code:
            logging.warning("winrm cat failed %r", result)
        return result.std_out

    # TODO(gz): Unlike SSHRemote.copy this only supports copying files, not
    #           directories and their content. Both the powershell script and
    #           the unpacking method will need updating to support that.
    def copy(self, destination_dir, source_globs):
        """Copy files from the remote machine."""
        # Encode globs into script to run on remote machine and return result.
        script = _ps_copy_script % ",".join(s.join('""') for s in source_globs)
        result = self.run_ps(script)
        if result.status_code:
            logging.warning("winrm copy stderr:\n%s", result.std_err)
            raise subprocess.CalledProcessError(result.status_code,
                                                "powershell", result)
        self._encoded_copy_to_dir(destination_dir, result.std_out)

    @staticmethod
    def _encoded_copy_to_dir(destination_dir, output):
        """Write remote files from powershell script to disk.

        The given output from the powershell script is one line per file, with
        the filename first, then a pipe, then the base64 encoded deflated file
        contents. This method reverses that process and creates the files in
        the given destination_dir.
        """
        start = 0
        while True:
            end = output.find("\n", start)
            if end == -1:
                break
            mid = output.find("|", start, end)
            if mid == -1:
                if not output[start:end].rstrip("\r\n"):
                    break
                raise ValueError("missing filename in encoded copy data")
            filename = output[start:mid]
            if "/" in filename:
                # Just defense against path traversal bugs, should never reach.
                raise ValueError("path not filename {!r}".format(filename))
            with open(os.path.join(destination_dir, filename), "wb") as f:
                f.write(zlib.decompress(output[mid + 1:end].decode("base64"),
                                        -zlib.MAX_WBITS))
            start = end + 1
