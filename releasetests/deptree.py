#!/usr/bin/python
"""Script for checking if a directory tree matches a dependencies list."""

from __future__ import print_function

import argparse
import errno
import os
import subprocess
import sys
import tempfile


__metaclass__ = type


class Dependency:
    """A GO Deps package dependency."""

    @classmethod
    def from_option(cls, option_value):
        return cls.from_string(option_value, ':')

    @classmethod
    def from_line(cls, line):
        return cls.from_string(line, '\t')

    @classmethod
    def from_string(cls, string, delimiter):
        parts = string.split(delimiter)
        if len(parts) < 3 or len(parts) > 4:
            raise argparse.ArgumentTypeError(
                'Expected form of package{delim}vcs{delim}rev'.format(
                    delim=delimiter))
        return cls(*parts)

    def __init__(self, package, vcs, revid, revno=None):
        self.package = package
        self.vcs = vcs
        self.revid = revid
        self.revno = revno or None

    def __eq__(self, other):
        return (
            isinstance(other, self.__class__) and
            self.__dict__ == other.__dict__)

    def __ne__(self, other):
        return not self == other

    def __repr__(self):
        return '<%s %s %s %s %s>' % (
            self.__class__.__name__,
            self.package, self.vcs, self.revid, self.revno)

    def __str__(self):
        return '%s\t%s\t%s\t%s' % (
            self.package, self.vcs, self.revid, self.revno or '')

    def to_line(self):
        return '%s\n' % str(self)


class DependencyFile:
    """A GO Deps dependencies.tsv created from several such files.

    The first dependencies.tsv is used as is, additional files may
    add dependencies, but not change those already defined.
    """

    def __init__(self, dep_files):
        self.dep_files = dep_files
        self.tmp_tsv = None
        self.deps, self.conflicts = self.consolidate_deps()

    def consolidate_deps(self):
        """Return a two-tuple of the deps dict and conflicts in the files.

        The dep_files is an list starting with the base set of deps, then
        overlayed with each successive file. If any package is redefined, it
        is added to conflicts.
        """
        deps = {}
        conflicts = []
        for dep_path in self.dep_files:
            with open(dep_path) as f:
                content = f.read()
            for line in content.splitlines():
                dep = Dependency(*line.split('\t'))
                if dep.package in deps and dep != deps[dep.package]:
                    conflicts.append((dep_path, dep))
                else:
                    deps[dep.package] = dep
        return deps, conflicts

    def include_deps(self, include):
        """Redefine or add additional deps to the tree."""
        redefined = []
        added = []
        for dep in include:
            if dep.package in self.deps:
                redefined.append(dep)
            else:
                added.append(dep)
            self.deps[dep.package] = dep
        return redefined, added

    def write_tmp_tsv(self):
        """Write the deps to a temp file, set tmp_tsv, and return its path.

        The caller of this method is resonsible for calling delete_tmp_tsv()
        when done.
        """
        fd, self.tmp_tsv = tempfile.mkstemp(
            suffix='.tsv', prefix='deptree', text=True)
        for package in sorted(self.deps.keys()):
            os.write(fd, self.deps[package].to_line())
        os.close(fd)
        return self.tmp_tsv

    def delete_tmp_tsv(self):
        """Delete tmp_tsv if it was written."""
        if self.tmp_tsv:
            try:
                os.unlink(self.tmp_tsv)
            except OSError as e:
                if e.errno != errno.ENOENT:
                    raise
            self.tmp_tsv = None
            return True
        return False

    def pin_deps(self):
        """Pin the tree to the current deps.

        This will write a temp dependencies tsv file, call godeps, then
        remove the file. the 'godeps' command must be in your path.
        """
        self.write_tmp_tsv()
        try:
            output = subprocess.check_output(['godeps', '-u', self.tmp_tsv])
        finally:
            self.delete_tmp_tsv()
        return output.strip()


def get_args(argv=None):
    """Return the argument parser for this program."""
    parser = argparse.ArgumentParser(
        "Pin a composite source tree to specific versions.")
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    parser.add_argument(
        '-i', '--include', action='append', default=[],
        help='Include an additional dependency. eg package:vcs:revision,')
    parser.add_argument(
        'dep_files', nargs='+',
        help='the dependencies.tsv files to merge')
    args = parser.parse_args(argv)
    args.include = [Dependency.from_option(o) for o in args.include]
    return args


def main(args=None):
    """Execute the commands from the command line."""
    exitcode = 0
    args = get_args(args)
    dep_file = DependencyFile(args.dep_files)
    if args.verbose and dep_file.conflicts:
        print('These conflicts were found:')
        for conflict in dep_file.conflicts:
            print(conflict)
    redefined, added = dep_file.include_deps(args.include)
    if args.verbose and redefined:
        print('These deps were redefined:')
        for dep in redefined:
            print(dep)
    if args.verbose and added:
        print('These deps were added:')
        for dep in added:
            print(dep)
    if not args.dry_run:
        output = dep_file.pin_deps()
    elif args.verbose:
        print('Not pinning deps because dry_run is true.')
    if args.verbose and output:
        print(output)
    return exitcode

if __name__ == '__main__':
    sys.exit(main())
