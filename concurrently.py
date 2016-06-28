#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import subprocess
import sys
import time
import traceback

__metaclass__ = type


class Task:

    def __init__(self, name_commdline):
        self.name, self.commandline = name_commdline.split('=', 1)
        self.command = self.commandline.split()
        self.out_log_name = '{}-out.log'.format(self.name)
        self.err_log_name = '{}-err.log'.format(self.name)
        self.proc = None
        self.returncode = None
        self._stdout = None
        self._stderr = None
        self._done = False

    def run(self):
        """Run a task in a subprocess and log std out and err."""
        self._open_logs()
        self.proc = subprocess.Popen(
            self.command, stdout=self._stdout, stderr=self._stderr)

    def _open_logs(self):
        """Open the tasks out and err logs for appending."""
        self._stdout = open(self.out_log_name, 'ab')
        self._stderr = open(self.err_log_name, 'ab')

    def _close_logs(self):
        """Close the tasks out and err logs."""
        self._stdout.close()
        self._stderr.close()

    def is_done(self):
        """Is the process complete.

        This call implicitly closes the std out and error logs.
        """
        if self.proc and self.proc.poll() is not None:
            self._close_logs()
            self.returncode = self.proc.returncode
            self._done = True
        return self._done

    def is_success(self):
        """Is the process successful."""
        return self.returncode == 0


class TaskManager:

    def __init__(self, tasks, concurrency=8):
        self.backlog = tasks
        self.concurrency = concurrency
        self.running = []
        self.complete = []

    def run(self):
        """Exec tasks in parallel."""
        while True:
            while self.backlog and len(self.running) < self.concurrency:
                task = self.backlog.pop()
                self.running.append(task)
                print(task.commandline)
                task.run()
            for task in self.running:
                if task.is_done():
                    self.running.remove(task)
                    self.complete.append(task)
            if not self.backlog and not self.running:
                break
            else:
                time.sleep(0.05)

    def get_returncode(self):
        """Return the sum of all return codes."""
        return sum([t.returncode for t in self.complete])


def parse_args(argv=None):
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
    returncode = 0
    args = parse_args(argv)
    tasks = [Task(t) for t in args.tasks]
    try:
        if args.verbose:
            names = [t.name for t in tasks]
            print('Running these tasks {}'.format(names))
        task_manager = TaskManager(tasks)
        task_manager.run()
        returncode = task_manager.get_returncode()
    except Exception as e:
        print(str(e))
        print(traceback.print_exc())
        return 2
    return returncode


if __name__ == '__main__':
    sys.exit(main())
