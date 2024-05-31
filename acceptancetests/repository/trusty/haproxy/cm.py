# Copyright 2010-2013 Canonical Ltd. All rights reserved.
import os
import re
import sys
import errno
import hashlib
import subprocess
import optparse

from os import curdir
from bzrlib.branch import Branch
from bzrlib.plugin import load_plugins
load_plugins()
from bzrlib.plugins.launchpad import account as lp_account

if 'GlobalConfig' in dir(lp_account):
    from bzrlib.config import LocationConfig as LocationConfiguration
    _ = LocationConfiguration
else:
    from bzrlib.config import LocationStack as LocationConfiguration
    _ = LocationConfiguration


def get_branch_config(config_file):
    """
    Retrieves the sourcedeps configuration for an source dir.
    Returns a dict of (branch, revspec) tuples, keyed by branch name.
    """
    branches = {}
    with open(config_file, 'r') as stream:
        for line in stream:
            line = line.split('#')[0].strip()
            bzr_match = re.match(r'(\S+)\s+'
                                 'lp:([^;]+)'
                                 '(?:;revno=(\\d+))?', line)
            if bzr_match:
                name, branch, revno = bzr_match.group(1, 2, 3)
                if revno is None:
                    revspec = -1
                else:
                    revspec = revno
                branches[name] = (branch, revspec)
                continue
            dir_match = re.match(r'(\S+)\s+'
                                 '\\(directory\\)', line)
            if dir_match:
                name = dir_match.group(1)
                branches[name] = None
    return branches


def main(config_file, parent_dir, target_dir, verbose):
    """Do the deed."""

    try:
        os.makedirs(parent_dir)
    except OSError as e:
        if e.errno != errno.EEXIST:
            raise

    branches = sorted(get_branch_config(config_file).items())
    for branch_name, spec in branches:
        if spec is None:
            # It's a directory, just create it and move on.
            destination_path = os.path.join(target_dir, branch_name)
            if not os.path.isdir(destination_path):
                os.makedirs(destination_path)
            continue

        (quoted_branch_spec, revspec) = spec
        revno = int(revspec)

        # qualify mirror branch name with hash of remote repo path to deal
        # with changes to the remote branch URL over time
        branch_spec_digest = hashlib.sha1(quoted_branch_spec).hexdigest()
        branch_directory = branch_spec_digest

        source_path = os.path.join(parent_dir, branch_directory)
        destination_path = os.path.join(target_dir, branch_name)

        # Remove leftover symlinks/stray files.
        try:
            os.remove(destination_path)
        except OSError as e:
            if e.errno != errno.EISDIR and e.errno != errno.ENOENT:
                raise

        lp_url = "lp:" + quoted_branch_spec

        # Create the local mirror branch if it doesn't already exist
        if verbose:
            sys.stderr.write('%30s: ' % (branch_name,))
            sys.stderr.flush()

        fresh = False
        if not os.path.exists(source_path):
            subprocess.check_call(['bzr', 'branch', '-q', '--no-tree',
                                   '--', lp_url, source_path])
            fresh = True

        if not fresh:
            source_branch = Branch.open(source_path)
            if revno == -1:
                orig_branch = Branch.open(lp_url)
                fresh = source_branch.revno() == orig_branch.revno()
            else:
                fresh = source_branch.revno() == revno

        # Freshen the source branch if required.
        if not fresh:
            subprocess.check_call(['bzr', 'pull', '-q', '--overwrite', '-r',
                                   str(revno), '-d', source_path,
                                   '--', lp_url])

        if os.path.exists(destination_path):
            # Overwrite the destination with the appropriate revision.
            subprocess.check_call(['bzr', 'clean-tree', '--force', '-q',
                                   '--ignored', '-d', destination_path])
            subprocess.check_call(['bzr', 'pull', '-q', '--overwrite',
                                   '-r', str(revno),
                                   '-d', destination_path, '--', source_path])
        else:
            # Create a new branch.
            subprocess.check_call(['bzr', 'branch', '-q', '--hardlink',
                                   '-r', str(revno),
                                   '--', source_path, destination_path])

        # Check the state of the destination branch.
        destination_branch = Branch.open(destination_path)
        destination_revno = destination_branch.revno()

        if verbose:
            sys.stderr.write('checked out %4s of %s\n' %
                             ("r" + str(destination_revno), lp_url))
            sys.stderr.flush()

        if revno != -1 and destination_revno != revno:
            raise RuntimeError("Expected revno %d but got revno %d" %
                               (revno, destination_revno))

if __name__ == '__main__':
    parser = optparse.OptionParser(
        usage="%prog [options]",
        description=(
            "Add a lightweight checkout in <target> for each "
            "corresponding file in <parent>."),
        add_help_option=False)
    parser.add_option(
        '-p', '--parent', dest='parent',
        default=None,
        help=("The directory of the parent tree."),
        metavar="DIR")
    parser.add_option(
        '-t', '--target', dest='target', default=curdir,
        help=("The directory of the target tree."),
        metavar="DIR")
    parser.add_option(
        '-c', '--config', dest='config', default=None,
        help=("The config file to be used for config-manager."),
        metavar="DIR")
    parser.add_option(
        '-q', '--quiet', dest='verbose', action='store_false',
        help="Be less verbose.")
    parser.add_option(
        '-v', '--verbose', dest='verbose', action='store_true',
        help="Be more verbose.")
    parser.add_option(
        '-h', '--help', action='help',
        help="Show this help message and exit.")
    parser.set_defaults(verbose=True)

    options, args = parser.parse_args()

    if options.parent is None:
        options.parent = os.environ.get(
            "SOURCEDEPS_DIR",
            os.path.join(curdir, ".sourcecode"))

    if options.target is None:
        parser.error(
            "Target directory not specified.")

    if options.config is None:
        config = [arg for arg in args
                  if arg != "update"]
        if not config or len(config) > 1:
            parser.error("Config not specified")
        options.config = config[0]

    sys.exit(main(config_file=options.config,
                  parent_dir=options.parent,
                  target_dir=options.target,
                  verbose=options.verbose))
