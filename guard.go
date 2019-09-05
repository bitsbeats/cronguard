package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/syslog"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/robfig/cron"
	"github.com/rs/xid"
	"golang.org/x/sync/errgroup"

	"github.com/bitsbeats/cronguard/errlog"
)

var (
	r               = regexp.MustCompile(`(?im)\b(err|fail|crit)`)
	name            = flag.String("name", "guard", "cron name in syslog")
	errFileName     = flag.String("errfile", "/var/log/cronstatus", "error report file")
	errFileQuiet    = flag.Bool("errfile-quiet", false, "hide timings in error report file")
	errFileHideUUID = flag.Bool("errfile-no-uuid", false, "hide uuid in error report file")
	quietTimes      = flag.String("quiet-times", "", "time ranges to ignore errors, format 'start(cron format):duration(golang duration):...")
	timeout         = flag.Duration("timeout", 0, "timeout for the cron")
	lockfile        = flag.String("lockfile", "", "lockfile to prevent the cron running twice")
)

func main() {
	// argparse
	log.SetFlags(0)
	flag.Parse()
	if !strings.HasPrefix(*name, "guard") {
		*name = fmt.Sprintf("guard.%s", *name)
	}
	if len(flag.Args()) != 1 {
		log.Fatal("please supply the cron as a single argument")
	}
	command := flag.Arg(0)
	UUID := xid.New()

	// open syslog
	slog, err := syslog.New(syslog.LOG_INFO|syslog.LOG_CRON, *name)
	if err != nil {
		log.Fatal(err)
	}
	defer slog.Close()

	// errfile
	errFile, err := os.OpenFile(*errFileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer errFile.Close()
	elog := errlog.New(!*errFileHideUUID, UUID.String(), errFile)

	// mixlog
	mixedlog := io.MultiWriter(elog, slog)

	// cancelation + timeout
	ctx, cancel := context.WithCancel(context.Background())
	if *timeout > time.Duration(0) {
		ctx, cancel = context.WithTimeout(context.Background(), *timeout)
	}
	defer cancel()
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		cancel()
	}()

	// run
	_, _, combined, exitCode, err := run(ctx, command, slog)

	// handle bad exit
	if err != nil && !isQuiet() {
		fmt.Fprintf(mixedlog, "errors while running %s\n", *name)
		if combined != nil {
			scanner := bufio.NewScanner(combined)
			for scanner.Scan() {
				line := scanner.Text()
				fmt.Fprintf(elog, "%s\n", line)
			}
		}
		fmt.Fprintf(mixedlog, "%s\n", err)
		fmt.Fprintf(mixedlog, "exit status %d\n", exitCode)
	}
}

func run(ctx context.Context, command string, slog io.Writer) (stdout *bytes.Buffer, stderr *bytes.Buffer, combined *bytes.Buffer, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	defer stdoutPipe.Close()
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return
	}
	defer stderrPipe.Close()
	err = cmd.Start()
	if err != nil {
		return
	}

	stdout = bytes.NewBuffer([]byte{})
	stderr = bytes.NewBuffer([]byte{})
	combined = bytes.NewBuffer([]byte{})
	w := io.MultiWriter(combined, slog, stdout)

	if !*errFileQuiet {
		start := time.Now()
		fmt.Fprintf(w, "cmd: %s\n", flag.Arg(0))
		fmt.Fprintf(w, "start: %s\n", start.Format(time.RFC3339))
		if _, ok := ctx.Deadline(); ok {
			fmt.Fprintf(w, "timeout: %s\n", timeout.String())
		}
		defer func() {
			end := time.Now()
			fmt.Fprintf(w, "end: %s\n", end.Format(time.RFC3339))
			fmt.Fprintf(w, "took: %s\n", end.Sub(start))
		}()
	}

	errgrp := errgroup.Group{}
	lock := sync.Mutex{}
	errgrp.Go(func() (err error) {
		return parseLog(&lock, stdoutPipe, combined, slog, stdout)
	})
	errgrp.Go(func() (err error) {
		return parseLog(&lock, stderrPipe, combined, slog, stderr)
	})

	err = cmd.Wait()
	if err != nil {
		exitCode = err.(*exec.ExitError).ExitCode()
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return
	}
	err = errgrp.Wait()
	if err != nil {
		return
	}
	if stderr.Len() > 0 {
		err = errors.New("stderr is not empty")
		return
	}
	return
}

func isQuiet() bool {
	if *quietTimes == "" {
		return false
	}
	ts := strings.Split(*quietTimes, ":")
	if len(ts)%2 != 0 {
		log.Fatalf("Invalid times.")
	}

	for i := 0; i < len(ts); i += 2 {
		startStr := ts[i]
		durStr := ts[i+1]
		now := time.Now()
		shed, err := cron.Parse(startStr)
		if err != nil {
			log.Fatalf("Unable to parse cron time: %s", err)
		}
		dur, err := time.ParseDuration(durStr)
		if err != nil {
			log.Fatalf("Unable to parse duration: %s", err)
		}
		start := shed.Next(now.Add(-dur))
		end := start.Add(dur)
		if now.After(start) && end.After(now) {
			return true
		}
	}
	return false
}

func parseLog(lock sync.Locker, r io.Reader, writers ...io.Writer) (err error) {
	var nonCritErr error
	w := io.MultiWriter(writers...)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Bytes()
		if nonCritErr == nil && !fine(line) {
			nonCritErr = fmt.Errorf("bad keyword in command output: %s", line)
		}
		line = append(line, '\n')

		lock.Lock()
		_, err = w.Write(line)
		lock.Unlock()
		if err != nil {
			return err
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	return nonCritErr
}

func fine(output []byte) (ok bool) {
	return !r.Match(output)
}
