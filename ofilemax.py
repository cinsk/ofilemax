#!/usr/bin/env python3

from os import listdir
from os.path import isdir, join, basename
from resource import prlimit, RLIMIT_NOFILE
from glob import glob
from getopt import getopt, GetoptError
import sys

noheader_mode = False
verbose_mode = False
program_name = basename(sys.argv[0])

def processes():
    return [ int(basename(f)) for f in glob("/proc/[0-9]*") if isdir(f) ]

def fdcount(pid):
    return len(listdir("/proc/%d/fd" % pid))

def sys_filemax():
    return int(open("/proc/sys/fs/file-max", "r").readline())

def print_entry(pid):
    (smax, hmax) = prlimit(pid, RLIMIT_NOFILE)
    curfd = fdcount(pid)
    print("%6d %6.2f%% %8d %8d %8d" % (pid, curfd / smax, curfd, smax, hmax))

def message(s):
    sys.stderr.write("%s: %s\n" % (program_name, s))
    sys.stderr.flush()
    
def main():
    global noheader_mode, verbose_mode
    
    try:
        opts, args = getopt(sys.argv[1:], "nv", ["help", "version", "verbose", "no-headers"])
    except GetoptError:
        message("invalid option")
        sys.exit(1)

    for o, a in opts:
        if o in ("-n", "--no-headers"):
            noheader_mode = True
        elif o in ("-v", "--verbose"):
            verbose_mode = True

    if not noheader_mode:
        try:
            print("# System Max Open Files (/proc/sys/fs/file-max): %d" % sys_filemax())
        except OSError as e:
            message("cannot read /proc/sys/fs/file-max: %s" % e)
        print("""\
# Check /etc/security/limits.conf per user's default limits
# Check /etc/sysctl.conf for system limits
#
#  PID   RATE% OPENFILE SOFT-MAX HARD-MAX
# ---- ------- -------- -------- --------""")

    for pid in processes():
        # print(pid)
        try:
            print_entry(pid)
        except OSError as e:
            if verbose_mode:
                message("cannot read open file information of pid %d" % pid)
    
if __name__ == '__main__':
    main()
