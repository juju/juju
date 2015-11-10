from argparse import ArgumentParser
import errno
import hashlib
import logging
import os
import re
import socket
import sys
import urlparse

from pipdeps import s3_auth_with_rc
from utility import configure_logging


def s3_download_files(s3_path, credential_path, dst_dir=None,
                      suffix=None, overwrite=False):
    """ Download files from S3 path.

    If dst_dir is set, it writes the S3 file to the directory. If the file
    already exists, it is overwritten if overwrite is set True or if the files
    md5 digest don't match.
    If suffix is specified, it only downloads files that match the suffix.
    """
    logging.info('Connecting to {}'.format(s3_path))
    conn = s3_auth_with_rc(credential_path)
    uri = urlparse.urlparse(s3_path)
    bucket = conn.get_bucket(uri.netloc)
    if dst_dir:
        mkdir_p(dst_dir)
    keys = bucket.list(uri.path.strip('/'))
    return download_files(keys, dst_dir, overwrite, suffix)


def filter_keys(keys, suffix):
    for key in keys:
        filename = os.path.basename(key.name)
        if not filename or (suffix and not filename.endswith(suffix)):
            continue
        yield key, filename


def download_files(keys, dst_dir=None, overwrite=False, suffix=None):
    remote_files = []
    if dst_dir:
        local_files = os.listdir(dst_dir)
    for key, filename in filter_keys(keys, suffix):
        remote_files.append(key.name)
        if dst_dir:
            dst_path = os.path.join(dst_dir, filename)
            if overwrite:
                logging.info("Overwriting the local file: {}".format(dst_path))
                _download(key, dst_path)
            elif filename not in local_files:
                logging.info("Matching file not found: {}.".format(dst_path))
                _download(key, dst_path)
            elif key.etag.strip('"') != get_md5(dst_path):
                logging.info("Existing file mismatching: {}.".format(dst_path))
                _download(key, dst_path)
            else:
                remote_files.remove(key.name)
                logging.info("Matching local file found: {}".format(dst_path))
    return remote_files


def _download(key, dst_path, retries=3):
    logging.info('Copying file: {} -> {}'.format(key.name, dst_path))
    for x in range(retries):
        try:
            if x != 0:
                logging.info('Retrying download due to a socket failure.')
            return key.get_contents_to_filename(dst_path)
        except socket.error as e:
            if e.errno != errno.ECONNRESET:
                raise
    else:
        raise


def get_md5(filename, size=65536):
    """Gets md5 hex digest of contents of given filename."""
    # Copied from Juju Report
    md5 = hashlib.md5()
    with open(filename, "rb") as f:
        while True:
            chunk = f.read(size)
            if not chunk:
                return md5.hexdigest()
            md5.update(chunk)


def get_os_str(platform):
    return {'win32': 'win', 'darwin': 'osx'}[platform]


def download_released_juju(args):
    logging.info('Getting released Juju for {}.'.format(args.platform))
    os_str = get_os_str(args.platform)
    dst_dir = os.path.join(os.environ['HOME'], 'old-juju', os_str)
    s3_path = "s3://juju-qa-data/client-archive/{}/".format(os_str)
    s3_download_files(
        s3_path, credential_path=args.credential_path, dst_dir=dst_dir)


def download_candidate_juju(args):
    logging.info('Getting candidate Juju for {}.'.format(args.platform))
    os_str = get_os_str(args.platform)
    dst_dir = os.path.join(os.environ['HOME'], 'candidate', os_str)
    file_ext = '.exe' if os_str == 'win' else '.tar.gz'
    for rev in args.revision:
        s3_path = ("s3://juju-qa-data/juju-ci/products/version-{}/build-{}-"
                   "client/".format(rev, os_str))
        builds = s3_download_files(
            s3_path, credential_path=args.credential_path, suffix=file_ext)
        if not builds:
            raise ValueError('Build revision not found: {}'.format(rev))
        build = select_build(builds)
        s3_download_files(
            build, credential_path=args.credential_path,
            suffix=file_ext, dst_dir=dst_dir)


def select_build(builds):
    """ Select greater build number. """
    #  Builds for the same revision have the following format:
    # ['juju-ci/products/version-3000/build-osx-client/build-838/client
    # /juju-1.25-alpha1-osx.tar.gz'],['juju-ci/products/version-3000/
    # build-osx-client/build-839/client/juju-1.25-alpha1-osx.tar.gz']
    build = max(
        builds, key=lambda x: int(re.search(r'build-(\d+)/', x).group(1)))
    build = build.split('/')[:5]
    return 's3://juju-qa-data/{}'.format('/'.join(build))


def parse_args(args=None):
    parser = ArgumentParser("Download released and candidate Juju.")
    parser.add_argument('credential_path',
                        help="Directory path to AWS credential.")
    parser.add_argument('-r', '--released', action='store_true',
                        help="Download released Juju.")
    parser.add_argument('-c', '--revision', nargs='+',
                        metavar='candidate revisions',
                        help="Download candidate Juju.")
    parser.add_argument('-v', '--verbose', action='count', default=0,
                        help="Increase verbosity of console output.")
    parser.add_argument('-p', '--platform', help="OS platform.",
                        default=sys.platform)
    parsed_args = parser.parse_args(args)
    return parsed_args


def mkdir_p(path):
    try:
        os.makedirs(path)
    except OSError as e:
        if e.errno == errno.EEXIST and os.path.isdir(path):
            pass
        else:
            raise


def main():
    args = parse_args()
    configure_logging(max(logging.WARNING - 10 * args.verbose, logging.DEBUG))
    if args.released:
        download_released_juju(args)
    if args.revision:
        download_candidate_juju(args)


if __name__ == '__main__':
    main()
