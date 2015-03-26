#!/usr/bin/python
"""Script for checking if a directory tree matches a dependencies list."""

from __future__ import print_function

import argparse
import os
import sys
import tempfile


class Dependency:

    @staticmethod
    def from_option(option_value):
        parts = option_value.split(':')
        if len(parts) < 3 or len(parts) > 4:
            raise argparse.ArgumentTypeError(
                'Expected form of package:vcs:rev')
        return Dependency(*parts)

    def __init__(self, package, vcs, revid, revno=None):
        self.package = package
        self.vcs = vcs
        self.revid = revid
        self.revno = revno or None

    def __eq__(self, other):
        return (
            isinstance(other, self.__class__)
            and self.__dict__ == other.__dict__)

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

    def __init__(self, dep_files, verbose=False):
        self.dep_files = dep_files
        self.verbose = verbose
        self.dep_path = None
        self.deps, self.conflicts = self.consolidate_deps()

    def consolidate_deps(self):
        """Return a two-tuple of the deps dict and conflicts in the files.

        The dep_files lis an list starting with the base set of deps, then
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
                    if self.verbose:
                        print('%s redefines %s' % (dep_path, dep))
                else:
                    deps[dep.package] = dep
        return deps, conflicts

    def include_deps(self, include):
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
        """Write the deps to a temp file and return its path.

        The caller of this function is resonsible for deleting the file
        when done.
        """
        fd, self.dep_path = tempfile.mkstemp(
            suffix='.tsv', prefix='deptree', text=True)
        for package in sorted(self.deps.keys()):
            os.write(fd, self.deps[package].to_line())
        os.close(fd)
        return self.dep_path


def main(args=None):
    """Execute the commands from the command line."""
    exitcode = 0
    args = get_args(args)
    dep_file = DependencyFile(args.dep_files, verbose=args.verbose)
    redefined, added = dep_file.include_deps(args.include)
    consolidated_tsv = dep_file.write_tmp_tsv()
    try:
        pass
    finally:
        if os.path.isfile(consolidated_tsv):
            os.unlink(consolidated_tsv)
    return exitcode


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
    parser.add_argument('srcdir', help='The src dir.')
    parser.add_argument(
        'dep_files', nargs='+',
        help='the dependencies.tsv files to merge')
    args = parser.parse_args(argv)
    args.include = [Dependency.from_option(o) for o in args.include]
    return args


if __name__ == '__main__':
    sys.exit(main())
