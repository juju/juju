#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
from contextlib import contextmanager
import subprocess
import sys
import traceback

__metaclass__ = type


class Task:

    def __init__(self, name_commdline):
        self.name, self.commandline = name_commdline.split('=', 1)
        self.command = self.commandline.split()
        self.out_log_name = '{}-out.log'.format(self.name)
        self.err_log_name = '{}-err.log'.format(self.name)
        self.returncode = None

    @contextmanager
    def run(self):
        """Yield the running proc, then wait to set the returncode."""
        with open(self.out_log_name, 'ab') as out_log:
            with open(self.err_log_name, 'ab') as err_log:
                try:
                    proc = subprocess.Popen(
                        self.command, stdout=out_log, stderr=err_log)
                    yield proc
                finally:
                    self.returncode = proc.wait()


def run_all(tasks):
    """Run all tasks in the list.

    The list is a queue that will be emptied.
    """
    try:
        task = tasks.pop()
    except IndexError:
        return
    with task.run():
        run_all(tasks)


def summarise_tasks(tasks, verbose=False):
    """Return of sum of tasks returncodes."""
    returncode = sum([t.returncode for t in tasks])
    if verbose:
        if returncode == 0:
            print('SUCCESS')
        else:
            print('FAIL')
            for task in tasks:
                if task.returncode != 0:
                    print('  {} failed with {}'.format(
                          task.name, task.returncode))
    return returncode


def parse_args(argv=None):
    """Return the parsed args for this program."""
    parser = ArgumentParser(
        description="Run many processes concurrently.")
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    parser.add_argument(
        'tasks', nargs='+', default=[],
        help="one or more tasks to run in the form of name='cmc -opt arg'.")
    return parser.parse_args(argv)


def main(argv=None):
    """Run go test against the content of a tarfile."""
    returncode = 254
    args = parse_args(argv)
    tasks = [Task(t) for t in args.tasks]
    try:
        if args.verbose:
            names = [t.name for t in tasks]
            print('Running these tasks {}'.format(names))
        run_all(list(tasks))
        returncode = summarise_tasks(tasks, verbose=args.verbose)
    except Exception as e:
        print(str(e))
        print(traceback.print_exc())
        return 253
    return returncode


if __name__ == '__main__':
    sys.exit(main())
