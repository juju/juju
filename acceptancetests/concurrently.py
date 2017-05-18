#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
from contextlib import contextmanager
import logging
import os
import subprocess
import shlex
import sys

from utility import configure_logging


__metaclass__ = type


log = logging.getLogger("concurrently")


def task_definition(name_commandline):
    name, commandline = name_commandline.split('=', 1)
    command = shlex.split(commandline)
    return name, command


class Task:

    def __init__(self, name, command, log_dir='.'):
        self.name = name
        self.command = command
        self.log_name = os.path.join(log_dir, '{}.log'.format(self.name))
        self.returncode = None
        self.proc = None

    @classmethod
    def from_arg(cls, name_commandline, log_dir='.'):
        return cls(*task_definition(name_commandline), log_dir=log_dir)

    def __eq__(self, other):
        if type(self) != type(other):
            return False
        return (self.name == other.name and
                self.command == other.command and
                self.log_name == other.log_name)

    @contextmanager
    def start(self):
        """Yield the running proc, then wait to set the returncode."""
        with open(self.log_name, 'ab') as out_log:
            self.proc = subprocess.Popen(
                self.command, stdout=out_log, stderr=out_log)
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
    """Log summary of results and returns the number of tasks that failed."""
    failed_count = sum(t.returncode != 0 for t in tasks)
    if not failed_count:
        log.debug('SUCCESS')
    else:
        log.debug('FAIL')
        for task in tasks:
            if task.returncode != 0:
                log.error('{} failed with {}\nSee {}'.format(
                          task.name, task.returncode, task.log_name))
    return failed_count


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
        'tasks', nargs='+', default=[], type=task_definition,
        help="one or more tasks to run in the form of name='cmc -opt arg'.")
    return parser.parse_args(argv)


def main(argv=None):
    """Run many tasks concurrently."""
    args = parse_args(argv)
    configure_logging(args.verbose)
    tasks = [Task(*t, log_dir=args.log_dir) for t in args.tasks]
    try:
        names = [t.name for t in tasks]
        log.debug('Running these tasks {}'.format(names))
        run_all(list(tasks))
    except Exception:
        log.exception("Script failed while running tasks")
        return 126
    return min(100, summarise_tasks(tasks))


if __name__ == '__main__':
    sys.exit(main())
