#!/usr/bin/python
"""Script for building source and binary debain packages."""

from __future__ import print_function

from argparse import ArgumentParser
from collections import namedtuple
import os
import shutil
import subprocess
import sys


DEFAULT_SPB = 'lp:~juju-qa/juju-release-tools/packaging-juju-core-default'


SourceFile = namedtuple('SourceFile', ['sha256', 'size', 'name', 'path'])


CREATE_LXC_TEMPLATE = """\
set -eu
sudo lxc-create -t download -n {container} -- -d ubuntu -r {series} -a {arch}
sudo mkdir /var/lib/lxc/{container}/rootfs/workspace
echo "lxc.mount.entry = {build_dir} workspace none bind 0 0" |
    sudo tee -a /var/lib/lxc/{container}/config
"""


BUILD_DEB_TEMPLATE = """\
sudo lxc-attach -n {container} -- bash <<"EOT"
    set -eu
    echo "\nInstalling common build deps.\n"
    cd workspace
    while ! ifconfig | grep -q "addr:10.0."; do
        echo "Waiting for network"
        sleep 1
    done
    set +e
    # Adding the ppa directly to sources.list without the archive key
    # requires apt to be run with --force-yes
    echo "{ppa}" >> /etc/apt/sources.list
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y --force-yes build-essential devscripts equivs
EOT
sudo lxc-attach -n {container} -- bash <<"EOT"
    set -eux
    echo "\nInstalling build deps from dsc.\n"
    cd workspace
    export DEBIAN_FRONTEND=noninteractive
    mk-build-deps -i --tool 'apt-get --yes --force-yes' *.dsc
EOT
sudo lxc-attach -n {container} -- bash <<"EOT"
    set -eux
    echo "\nBuilding the packages.\n"
    cd workspace
    rm *build-deps*.deb || true
    dpkg-source -x *.dsc
    cd $(basename *.orig.tar.gz .orig.tar.gz | tr _ -)
    dpkg-buildpackage -us -uc
EOT
"""


def parse_dsc(dsc_path, verbose=False):
    """Return the source files need to build a binary package."""
    there = os.path.dirname(dsc_path)
    dsc_name = os.path.basename(dsc_path)
    dcs_source_file = SourceFile(None, None, dsc_name, dsc_path)
    files = [dcs_source_file]
    with open(dsc_path) as f:
        content = f.read()
    found = False
    for line in content.splitlines():
        if found and line.startswith(' '):
            data = line.split()
            data.append(os.path.join(there, data[2]))
            files.append(SourceFile(*data))
            if verbose:
                print("Found %s" % files[-1].name)
        elif found:
            # All files were found.
            break
        if not found and line.startswith('Checksums-Sha256:'):
            found = True
    return files


def setup_local(location, series, arch, source_files, verbose=False):
    """Create a directory to build binaries in.

    The directoy has the source files required to build binaries.
    """
    build_dir = os.path.abspath(
        os.path.join(location, 'juju-build-{}-{}'.format(series, arch)))
    if verbose:
        print('Creating %s' % build_dir)
    os.makedirs(build_dir)
    for sf in source_files:
        dest_path = os.path.join(build_dir, sf.name)
        if verbose:
            print('Copying %s to %s' % (sf.name, build_dir))
        shutil.copyfile(sf.path, dest_path)
    return build_dir


def setup_lxc(series, arch, build_dir, verbose=False):
    """Create an LXC container to build binaries.

    The local build_dir with the source files is bound to the container.
    """
    container = '{}-{}'.format(series, arch)
    lxc_script = CREATE_LXC_TEMPLATE.format(
        container=container, series=series, arch=arch, build_dir=build_dir)
    if verbose:
        print('Creating %s container' % container)
    output = subprocess.check_output([lxc_script], shell=True)
    if verbose:
        print(output)
    return container


def build_in_lxc(container, build_dir, ppa=None, verbose=False):
    """Build the binaries from the source files in the container."""
    returncode = 1
    if ppa:
        path = ppa.split(':')[1]
        series = container.split('-')[0]
        ppa = 'deb http://ppa.launchpad.net/{}/ubuntu {} main'.format(
            path, series)
    else:
        ppa = '# No PPA added.'
    # The work in the container runs as a different user. Care is needed to
    # ensure permissions and ownership are correct before and after the build.
    os.chmod(build_dir, 0o777)
    subprocess.check_call(['sudo', 'lxc-start', '-d', '-n', container])
    try:
        build_script = BUILD_DEB_TEMPLATE.format(container=container, ppa=ppa)
        returncode = subprocess.call([build_script], shell=True)
    finally:
        subprocess.check_call(['sudo', 'lxc-stop', '-n', container])
        user = os.environ.get('USER', 'jenkins')
        subprocess.check_call(['sudo', 'chown', '-R', user, build_dir])
        os.chmod(build_dir, 0o775)
    return returncode


def teardown_lxc(container, verbose=False):
    """Destroy the lxc container."""
    if verbose:
        print('Deleting the lxc container %s' % container)
    subprocess.check_call(['sudo', 'lxc-destroy', '-n', container])


def move_debs(build_dir, location, verbose=False):
    """Move the debs from the build_dir to the location dir."""
    found = False
    files = [f for f in os.listdir(build_dir) if f.endswith('.deb')]
    for file_name in files:
        file_path = os.path.join(build_dir, file_name)
        dest_path = os.path.join(location, file_name)
        if verbose:
            print("Found %s" % file_name)
        shutil.move(file_path, dest_path)
        found = True
    return found


def build_binary(dsc_path, location, series, arch, ppa=None, verbose=False):
    """Build binary debs from a dsc file."""
    # If location is remote, setup remote location and run.
    source_files = parse_dsc(dsc_path, verbose=verbose)
    build_dir = setup_local(
        location, series, arch, source_files, verbose=verbose)
    container = setup_lxc(series, arch, build_dir, verbose=verbose)
    try:
        build_in_lxc(container, build_dir, ppa=ppa, verbose=verbose)
    finally:
        teardown_lxc(container, verbose=False)
    move_debs(build_dir, location, verbose=verbose)
    return 0


def build_source(tar_file, location, series, bugs,
                 debemail=None, debfullname=None, gpgcmd=None,
                 branch=None, upatch=None, verbose=False):
    pass


def main(argv):
    """Execute the commands from the command line."""
    exitcode = 0
    args = get_args(argv)
    if args.command == 'source':
        exitcode = build_source(
            args.tar_file, args.location, args.series, args.bugs,
            debemail=args.debemail, debfullname=args.debfullname,
            gpgcmd=args.gpgcmd, branch=args.branch, upatch=args.upatch,
            verbose=args.verbose)
    if args.command == 'binary':
        exitcode = build_binary(
            args.dsc, args.location, args.series, args.arch,
            ppa=args.ppa, verbose=args.verbose)
    return exitcode


def get_args(argv=None):
    """Return the arguments for this program."""
    parser = ArgumentParser("Build debian packages.")
    parser.add_argument(
        "-v", "--verbose", action="store_true", default=False,
        help="Increase the verbosity of the output")
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    src_parser = subparsers.add_parser('source', help='Build source packages')
    src_parser.add_argument(
        '--debemail', default=os.environ.get("DEBEMAIL"),
        help="Your email address; Environment: DEBEMAIL.")
    src_parser.add_argument(
        '--debfullname', default=os.environ.get("DEBFULLNAME"),
        help="Your full name; Environment: DEBFULLNAME.")
    src_parser.add_argument(
        '--gpgcmd', default='/usr/bin/gpg',
        help="Path to an alternate gpg signing command; default /usr/bin/gpg.")
    src_parser.add_argument(
        '--branch', default=DEFAULT_SPB,
        help="The base/previous source package branch.")
    src_parser.add_argument(
        '--upatch', default=0, help="The Ubuntu patch number.")
    src_parser.add_argument('tar_file', help="The release tar file.")
    src_parser.add_argument("location", help="The location to build in.")
    src_parser.add_argument(
        'series', help="The destination Ubuntu release or LIVING for all.")
    src_parser.add_argument(
        'bugs', nargs='*', help="Bugs this version will fix in the release.")
    bin_parser = subparsers.add_parser('binary', help='Build a binary package')
    bin_parser.add_argument(
        '--ppa', default=None, help="The PPA that provides package deps.")
    bin_parser.add_argument("dsc", help="The dsc file to build")
    bin_parser.add_argument("location", help="The location to build in.")
    bin_parser.add_argument("series", help="The series to build in.")
    bin_parser.add_argument("arch", help="The dpkg architure to build in.")
    args = parser.parse_args(argv[1:])
    return args


if __name__ == '__main__':
    sys.exit(main(sys.argv))
