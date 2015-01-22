#!/usr/bin/python
"""Script for checking if a directory tree matches a dependencies list."""

from __future__ import print_function

import argparse
import os
import sys
import tempfile


class Dependency:

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


def consolidate_deps(dep_files, verbose=False):
    """Return a two-tuple of the dependency dict and conflicts in the files.

    The dep_files lis an list starting with the base set of deps, then
    overlayed with each successive file. If any package is redefined, it is
    added to conflicts.
    """
    deps = {}
    conflicts = []
    for dep_path in dep_files:
        with open(dep_path) as f:
            content = f.read()
        for line in content.splitlines():
            dep = Dependency(*line.split('\t'))
            if dep.package in deps and dep != deps[dep.package]:
                conflicts.append((dep_path, dep))
                if verbose:
                    print('%s redefines %s' % (dep_path, dep))
            else:
                deps[dep.package] = dep
    return deps, conflicts


def write_tmp_tsv(deps, verbose=False):
    """Write the deps to a temp file and return its path.

    The caller of this function is resonsible for deleting the file when done.
    """
    fd, dep_path = tempfile.mkstemp(suffix='.tsv', prefix='deptree', text=True)
    for package in sorted(deps.keys()):
        os.write(fd, deps[package].to_line())
    os.close(fd)
    return dep_path


def main(args=None):
    """Execute the commands from the command line."""
    exitcode = 0
    args = get_args(args)
    deps, conflicts = consolidate_deps(args.dep_files, verbose=args.verbose)
    consolidated_tsv = write_tmp_tsv(deps)
    try:
        pass
    finally:
        if os.path.isfile(consolidated_tsv):
            os.unlink(consolidated_tsv)
#    deps = get_dependencies(args.depfile)
#    known = deps.union(args.ignore)
#    present, unknown = compare_dependencies(known, args.srcdir)
#    missing = deps.difference(present)
#    if missing:
#        print("Given dependencies missing:\n {}".format("\n ".join(missing)))
#        exitcode = 1
#    if unknown:
#        print("Extant directories unknown:\n {}".format("\n ".join(unknown)))
#        exitcode = 1
    return exitcode


def get_args(args=None):
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
        help='Include an additional dependency. eg for/bar:git:123abc0:')
    parser.add_argument('srcdir', help='The src dir.')
    parser.add_argument(
        'dep_files', nargs='+',
        help='the dependencies.tsv files to merge')
    return parser.parse_args(args)


if __name__ == '__main__':
    sys.exit(main())
