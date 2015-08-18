#!/usr/bin/env python
from argparse import ArgumentParser
from itertools import chain
import signal
import subprocess
import sys
import time

from utility import until_timeout


# Generate a list of all valid signals for this platform
signals = dict(
    (x[3:], getattr(signal, x)) for x in dir(signal) if
    x.startswith('SIG') and x not in ('SIG_DFL', 'SIG_IGN'))


def parse_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('duration', type=float)

    parser.add_argument(
        '--signal', default='TERM', choices=sorted(signals.keys()))
    return parser.parse_known_args(argv)


def run_command(duration, signal, command):
    proc = subprocess.Popen(command)
    for remaining in chain([None], until_timeout(duration)):
        result = proc.poll()
        if result is not None:
            return result
        time.sleep(0.1)
    else:
        proc.send_signal(signal)
        return 124


def main(args=None):
    args, command = parse_args(args)
    return run_command(args.duration, signals[args.signal], command)


if __name__ == '__main__':
    sys.exit(main())
