#!/usr/bin/env python
from argparse import ArgumentParser
from datetime import (
    datetime,
    timedelta,
    )
from operator import getitem
import signal
import subprocess
import sys
import time

signals = dict((x[3:], getattr(signal, x)) for x in dir(signal) if
        x.startswith('SIG') and x not in ('SIG_DFL', 'SIG_IGN'))

def parse_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('duration', type=float)

    parser.add_argument(
        '--signal', default='TERM', choices=sorted(signals.keys()))
    return parser.parse_known_args(argv)


def run_command(duration, signal, command):
    proc = subprocess.Popen(command)
    start = datetime.now()
    end = start + timedelta(seconds=duration)
    while True:
        result = proc.poll()
        if result is not None:
            return result
        time.sleep(0.1)
        if datetime.now() > end:
            proc.send_signal(signal)
            return 124


def main(args=None):
    args, command = parse_args(args)
    return run_command(args.duration, signals[args.signal], command)


if __name__ == '__main__':
    sys.exit(main())
