package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"

	"github.com/pborman/getopt/v2"
)

const RLIMIT_INFINITY = ^uint64(0)

var msg *log.Logger
var version_mode, help_mode, noheader_mode, verbose_mode bool
var ratio_min_threshold, ratio_max_threshold uint
var soft_min_threshold, soft_max_threshold uint64
var hard_min_threshold, hard_max_threshold uint64

func init() {
	msg = log.New(os.Stderr, "...", 0)
	msg.SetPrefix(filepath.Base(os.Args[0]) + ": ")

	ratio_min_threshold = 0
	ratio_max_threshold = 101

	soft_min_threshold = 0
	hard_min_threshold = 0
	soft_max_threshold = math.MaxUint64
	hard_max_threshold = math.MaxUint64

	getopt.FlagLong(&noheader_mode, "no-headers", 'n', "Print no header line at all.")
	getopt.FlagLong(&verbose_mode, "verbose", 'v', "Print additional (warning) messages if any.")

	getopt.FlagLong(&ratio_min_threshold, "ratio-min", 'r', "lower threshold for the open file ratio")
	getopt.FlagLong(&ratio_max_threshold, "ratio-max", 'R', "higher threshold for the open file ratio")
	getopt.FlagLong(&soft_min_threshold, "soft-min", 's', "lower threshold for the open file soft limit")
	getopt.FlagLong(&soft_max_threshold, "soft-max", 'S', "higher threshold for the open file soft limit")
	getopt.FlagLong(&hard_min_threshold, "hard-min", 'h', "lower threshold for the open file soft limit")
	getopt.FlagLong(&hard_max_threshold, "hard-max", 'H', "higher threshold for the open file soft limit")

	getopt.FlagLong(&version_mode, "version", 0, "display version information")
	getopt.FlagLong(&help_mode, "help", 1, "show usage")
}

func pfdcount(pid int) (int, error) {
	path := fmt.Sprintf("/proc/%d/fd", pid)

	f, err := os.Open(path)
	if err != nil {
		return -1, err
	}
	files, err := f.Readdirnames(-1)
	if err != nil {
		return -1, err
	}

	return len(files), nil
}

func sys_filemax() uint64 {
	var fmax uint64 = math.MaxUint64

	f, err := os.Open("/proc/sys/fs/file-max")
	if err != nil {
		return fmax
	}
	rd := bufio.NewReader(f)
	line, err := rd.ReadString('\n')
	if err != nil {
		return fmax
	}
	fmax, err = strconv.ParseUint(line, 10, 64)
	if err != nil {
		return math.MaxInt64
	}
	return fmax
}

func pfdlimit(pid int) (*syscall.Rlimit, error) {
	// There is syscall.Prlimit() at the current version
	var rlim syscall.Rlimit
	delims := regexp.MustCompile("[ \n\r]+")

	pfile := fmt.Sprintf("/proc/%d/limits", pid)
	f, err := os.OpenFile(pfile, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rd := bufio.NewReader(f)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		tokens := delims.Split(line, -1)
		if tokens[0] == "Max" && tokens[1] == "open" && tokens[2] == "files" {
			if tokens[3] == "unlimited" {
				rlim.Cur = RLIMIT_INFINITY
			} else {
				rlim.Cur, err = strconv.ParseUint(tokens[3], 10, 64)
				if err != nil {
					return nil, err
				}
			}
			if tokens[4] == "unlimited" {
				rlim.Max = RLIMIT_INFINITY
			} else {
				rlim.Max, err = strconv.ParseUint(tokens[4], 10, 64)
				if err != nil {
					return nil, err
				}
			}
			break
		}
	}
	return &rlim, nil
}

func processes() ([]int, error) {
	r := regexp.MustCompile("^[0-9]+$")

	pids := make([]int, 0, 16)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		finfo, err := e.Info()
		if err != nil {
			continue
		}
		if !finfo.IsDir() {
			continue
		}
		if !r.MatchString(e.Name()) {
			continue
		}
		pid, _ := strconv.Atoi(e.Name())
		pids = append(pids, pid)
	}
	return pids, nil
}

func printEntry(pid int, rlim syscall.Rlimit, sysmax uint64) error {
	// fmt.Printf("%s  %10.6f%% %d/%s  %s\n", e.Name(), ratio, nfiles, c, m)
	// .....
	nfiles, err := pfdcount(pid)
	if err != nil {
		return err
	}

	lim := rlim.Cur
	if lim == RLIMIT_INFINITY && sysmax > 0 {
		lim = sysmax
	}

	ratio := float64(nfiles) / float64(lim)

	if ratio >= float64(ratio_min_threshold) && ratio < float64(ratio_max_threshold) {
		// fmt.Printf("%6d %6.2f%% %-20s %-20s %20d\n", pid, ratio, c, m, sysmax)
		if rlim.Cur >= soft_min_threshold && rlim.Cur < soft_max_threshold {
			if rlim.Max >= hard_min_threshold && rlim.Max < hard_max_threshold {
				fmt.Printf("%6d %6.2f%% %8d %8d %8d\n", pid, ratio, nfiles, rlim.Cur, rlim.Max)
			}
		}
	}

	return nil
}

func main() {
	//msg.Printf("hello")
	getopt.Parse()
	// args := getopt.Args()

	if version_mode {
		msg.Println("version 0.1")
		os.Exit(0)
	}
	if help_mode {
		getopt.Usage()
		os.Exit(0)
	}

	pids, err := processes()
	if err != nil {
		msg.Fatal("cannot get the list of process")
	}

	fmax := sys_filemax()

	if !noheader_mode {
		fmt.Printf("# System Max Open Files: %v\n", fmax)
		fmt.Printf("#\n")
		fmt.Printf("#  PID   RATE%% OPENFILE SOFT-MAX HARD-MAX\n")
		fmt.Printf("# ---- ------- -------- -------- --------\n")
	}

	for _, pid := range pids {
		rlim, err := pfdlimit(pid)
		if err != nil {
			continue
		}
		err = printEntry(pid, *rlim, fmax)
		if verbose_mode && err != nil {
			msg.Printf("cannot read open file information of pid %d", pid)
		}
	}

}
