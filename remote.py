"""Remote helper class for communicating with juju machines."""

__metaclass__ = type


import logging
import subprocess


class Remote:
    """Remote represents a juju machine to access over the network."""

    _ssh_opts = [
        "-o", "User ubuntu",
        "-o", "UserKnownHostsFile /dev/null",
        "-o", "StrictHostKeyChecking no",
    ]

    timeout = "5m"

    def __init__(self, client=None, unit=None, address=None):
        if address is None and (client is None or unit is None):
            raise ValueError("Remote needs either address or client and unit")
        self.client = client
        self.unit = unit
        self.use_juju_ssh = unit is not None
        self.address = address

    def __repr__(self):
        params = []
        if self.client is not None:
            params.append("env=" + repr(self.client.env.environment))
        if self.unit is not None:
            params.append("unit=" + repr(self.unit))
        if self.address is not None:
            params.append("addr=" + repr(self.address))
        return "<Remote {}>".format(" ".join(params))

    def run(self, command):
        """Run a command on the remote machine."""
        if self.unit and self.use_juju_ssh:
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

    def _ensure_address(self):
        if self.address:
            return
        # TODO(gz): Really want something like this instead:
        # self.address = self.client.get_address_for_unit(self.unit)
        status = self.client.get_status()
        unit = status.get_unit(self.unit)
        self.address = unit['public-address']

    def _run_subprocess(self, command):
        if self.timeout:
            command = ["timeout", self.timeout] + command
        return subprocess.check_output(command)
