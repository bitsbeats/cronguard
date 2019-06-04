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

	"github.com/gofrs/uuid"
	"golang.org/x/sync/errgroup"

	errLog "github.com/bitsbeats/cronguard/elog"
)

var (
	r               = regexp.MustCompile("(?i)(err|fail|crit)")
	name            = flag.String("name", "general", "cron name in syslog")
	errFileName     = flag.String("errfile", "/var/log/cronstatus", "error report file")
	errFileHideUUID = flag.Bool("no-err-uuid", false, "hide uuid in error report file")
)

func main() {
	// argparse
	log.SetFlags(0)
	flag.Parse()
	if !strings.HasPrefix(*name, "cron.") {
		prefixed := fmt.Sprintf("cron.%s", *name)
		name = &prefixed
	}
	if len(flag.Args()) != 1 {
		log.Fatal("please supply the cron as a single argument")
	}
	command := flag.Arg(0)

	// open syslog
	UUID := uuid.Must(uuid.NewV4())
	slog, err := syslog.New(syslog.LOG_INFO, *name)
	if err != nil {
		log.Fatal(err)
	}

	// errfile
	defer slog.Close()
	errFile, err := os.OpenFile(*errFileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		log.Fatal(err)
	}
	elog := errLog.New(!*errFileHideUUID, UUID.String(), errFile)

	// run
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		cancel()
		elog.Printf("cancelled")
	}()
	_, _, combined, exitCode, err := run(ctx, command, slog)

	// handle good exit
	if err == nil {
		return
	}

	// handle bad exit
	elog.Printf("errors while running %s\n", *name)
	scanner := bufio.NewScanner(combined)
	for scanner.Scan() {
		line := scanner.Text()
		elog.Printf("%s\n", line)
	}
	elog.Printf("%s\n", err)
	elog.Printf("exitcode %d\n", exitCode)

}

func run(ctx context.Context, command string, logfile io.Writer) (stdout *bytes.Buffer, stderr *bytes.Buffer, combined *bytes.Buffer, exitCode int, err error) {
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
	errgrp.Go(func() (err error) {
		return parseLog(&lock, stdoutPipe, combined, logfile, stdout)
	})
	errgrp.Go(func() (err error) {
		return parseLog(&lock, stderrPipe, combined, logfile, stderr)
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
