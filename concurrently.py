#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
from contextlib import contextmanager
import logging
import os
import subprocess
import shlex
import sys
import traceback

from utility import configure_logging


__metaclass__ = type


log = logging.getLogger("concurrently")


class Task:

    def __init__(self, name_commdline, log_dir='.'):
        self.name, self.commandline = name_commdline.split('=', 1)
        self.command = shlex.split(self.commandline)
        self.out_log_name = os.path.join(
            log_dir, '{}-out.log'.format(self.name))
        self.err_log_name = os.path.join(
            log_dir, '{}-err.log'.format(self.name))
        self.returncode = None
        self.proc = None

    def __eq__(self, other):
        if type(self) != type(other):
            return False
        return (self.name == other.name and
                self.command == other.command and
                self.out_log_name == other.out_log_name and
                self.err_log_name == other.err_log_name)

    @contextmanager
    def start(self):
        """Yield the running proc, then wait to set the returncode."""
        with open(self.out_log_name, 'ab') as out_log:
            with open(self.err_log_name, 'ab') as err_log:
                self.proc = subprocess.Popen(
                    self.command, stdout=out_log, stderr=err_log)
                log.debug('Started {}'.format(self.name))
                yield self.proc

    def finish(self):
        log.debug('Waiting for {} to finish'.format(self.name))
        self.returncode = self.proc.wait()
        log.debug('{} finished'.format(self.name))


def run_all(tasks):
    """Run all tasks in the list.

    The list is a queue that will be emptied.
    """
    try:
        task = tasks.pop()
    except IndexError:
        return
    with task.start():
        run_all(tasks)
        task.finish()


def summarise_tasks(tasks):
    """Return of sum of tasks returncodes."""
    returncode = max([t.returncode for t in tasks])
    if returncode == 0:
        log.debug('SUCCESS')
    else:
        log.debug('FAIL')
        for task in tasks:
            if task.returncode != 0:
                log.error('{} failed with {}\nSee {}'.format(
                          task.name, task.returncode, task.err_log_name))
    return returncode


def parse_args(argv=None):
    """Return the parsed args for this program."""
    parser = ArgumentParser(
        description="Run many tasks concurrently.")
    parser.add_argument(
        '-v', '--verbose', action='store_const',
        default=logging.INFO, const=logging.DEBUG,
        help='Increase verbosity.')
    parser.add_argument(
        '-l', '--log_dir', default='.', type=os.path.expanduser,
        help='The path to store the logs for each task.')
    parser.add_argument(
        'tasks', nargs='+', default=[],
        help="one or more tasks to run in the form of name='cmc -opt arg'.")
    return parser.parse_args(argv)


def main(argv=None):
    """Run many tasks concurrently."""
    returncode = 254
    args = parse_args(argv)
    configure_logging(args.verbose)
    tasks = [Task(t, args.log_dir) for t in args.tasks]
    try:
        names = [t.name for t in tasks]
        log.debug('Running these tasks {}'.format(names))
        run_all(list(tasks))
        returncode = summarise_tasks(tasks)
    except Exception as e:
        log.error(str(e))
        log.error(traceback.print_exc())
        return 253
    return returncode


if __name__ == '__main__':
    sys.exit(main())
