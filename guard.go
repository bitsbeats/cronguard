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
	slog, err := syslog.New(syslog.LOG_INFO | syslog.LOG_CRON, *name)
	if err != nil {
		log.Fatal(err)
	}

	// errfile
	defer slog.Close()
	errFile, err := os.OpenFile(*errFileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		log.Fatal(err)
	}
	elog := errlog.New(!*errFileHideUUID, UUID.String(), errFile)

	// run
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		cancel()
	}()
	_, _, combined, exitCode, err := run(ctx, command, slog)

	// handle good exit
	if err == nil {
		return
	}

	// handle bad exit
	mixedlog := io.MultiWriter(elog, slog)
	fmt.Fprintf(mixedlog, "errors while running %s\n", *name)
	scanner := bufio.NewScanner(combined)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintf(elog, "%s\n", line)
	}
	fmt.Fprintf(mixedlog, "%s\n", err)
	fmt.Fprintf(mixedlog, "exit status %d\n", exitCode)

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

	errgrp := errgroup.Group{}
	stdout = bytes.NewBuffer(nil)
	stderr = bytes.NewBuffer(nil)
	combined = bytes.NewBuffer(nil)
	lock := sync.Mutex{}
	w := io.MultiWriter(combined, slog, stdout)

	if ! *errFileQuiet {
		start := time.Now()
		fmt.Fprintf(w, "start: %s\n", start.Format(time.RFC3339))
		fmt.Fprintf(w, "cmd: %s\n", flag.Arg(0))
		defer func() {
			end := time.Now()
			fmt.Fprintf(w, "end: %s\n", end.Format(time.RFC3339))
			fmt.Fprintf(w, "took: %s\n", end.Sub(start))
		}()
	}

	errgrp.Go(func() (err error) {
		return parseLog(&lock, stdoutPipe, combined, slog, stdout)
	})
	errgrp.Go(func() (err error) {
		return parseLog(&lock, stderrPipe, combined, slog, stderr)
	})

	err = errgrp.Wait()
	if err != nil {
		return
	}
	err = cmd.Wait()
	if err != nil {
		exitCode = err.(*exec.ExitError).ExitCode()
		return
	}
	if stderr.Len() > 0 {
		err = errors.New("stderr is not empty")
		return
	}
	return
}

func parseLog(lock sync.Locker, r io.Reader, writers ...io.Writer) (err error) {
	var nonCritErr error
	w := io.MultiWriter(writers...)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lock.Lock()
		line := append(scanner.Bytes(), '\n')

		if nonCritErr == nil && !fine(line) {
			nonCritErr = errors.New("bad keyword in command output")
		}

		_, err := w.Write(line)
		lock.Unlock()
		if err != nil {
			return err
		}
	}
	return nonCritErr
}

func fine(output []byte) (ok bool) {
	return !r.Match(output)
}
