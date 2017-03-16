#!/usr/bin/env python
"""A Python implementation of the *nix utility for use on all platforms."""
from argparse import ArgumentParser
from itertools import chain
import signal
import subprocess
import sys
import time

from utility import until_timeout


# Generate a list of all signals that can be used with Popen.send_signal on
# this platform.
if sys.platform == 'win32':
    signals = {
        'TERM': signal.SIGTERM,
        # CTRL_C_EVENT is also supposed to work, but experience shows
        # otherwise.
        'CTRL_BREAK': signal.CTRL_BREAK_EVENT,
        }
else:
    # Blech.  No equivalent of errno.errorcode for signals.
    signals = dict(
        (x[3:], getattr(signal, x)) for x in dir(signal) if
        x.startswith('SIG') and x not in ('SIG_DFL', 'SIG_IGN'))


def parse_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('duration', type=float)

    parser.add_argument(
        '--signal', default='TERM', choices=sorted(signals.keys()))
    return parser.parse_known_args(argv)


def run_command(duration, timeout_signal, command):
    """Run a subprocess.  If a timeout elapses, send specified signal.

    :param duration: Timeout in seconds.
    :param timeout_signal: Signal to send to the subprocess on timeout.
    :param command: Subprocess to run (Popen args).
    :return: exit status of the subprocess, 124 if the subprocess was
        signalled.
    """
    if sys.platform == 'win32':
        # support CTRL_BREAK
        creationflags = subprocess.CREATE_NEW_PROCESS_GROUP
    else:
        creationflags = 0
    proc = subprocess.Popen(command, creationflags=creationflags)
    for remaining in chain([None], until_timeout(duration)):
        result = proc.poll()
        if result is not None:
            return result
        time.sleep(0.1)
    else:
        proc.send_signal(timeout_signal)
        proc.wait()
        return 124


def main(args=None):
    args, command = parse_args(args)
    return run_command(args.duration, signals[args.signal], command)


if __name__ == '__main__':
    sys.exit(main())
