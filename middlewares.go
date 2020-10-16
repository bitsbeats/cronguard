package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"log/syslog"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"golang.org/x/sync/errgroup"
)

// setupLogs allocates buffers for combined, stdout and stderr. in addition it writes the errfile
func setupLogs(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		combined := bytes.NewBuffer([]byte{})
		cr.Status.Combined = combined
		cr.Status.Stdout = bytes.NewBuffer([]byte{})
		cr.Status.Stderr = bytes.NewBuffer([]byte{})
		errFile, errFileErr := os.OpenFile(cr.ErrFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
		if errFileErr != nil {
			log.Fatalf("error while opening %s: %s", cr.ErrFile, errFileErr)
		}
		defer errFile.Close()

		err = g(ctx, cr)

		if err != nil {
			n, err := combined.WriteTo(errFile)
			if n == 0 {
				return fmt.Errorf("no output for err file")
			}
			if err != nil {
				return fmt.Errorf("error while writing %s: %s", cr.ErrFile, err)
			}
		}
		return nil
	}
}

// writeSyslog copies combined log in realtime to syslog
func writeSyslog(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		slog, err := syslog.New(syslog.LOG_INFO|syslog.LOG_CRON, cr.Name)
		if err != nil {
			log.Fatal(err)
		}
		defer slog.Close()
		cr.Status.Combined = io.MultiWriter(slog, cr.Status.Combined)
		err = g(ctx, cr)
		return err
	}
}

// insertUUID prefixes each line with a uuid unless errfile-no-uuid flag is set
func insertUUID(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		if cr.ErrFileHideUUID {
			return g(ctx, cr)
		}
	        combined := newUUIDPrefixer(cr.Status.Combined)
		cr.Status.Combined = combined
		return g(ctx, cr)
	}
}

// combinelogs adds stderr and stdout to combined log
func combineLogs(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		combined := NewLockedWriter(cr.Status.Combined)
		cr.Status.Stdout = io.MultiWriter(cr.Status.Stdout, combined)
		cr.Status.Stderr = io.MultiWriter(cr.Status.Stderr, combined)
		err = g(ctx, cr)
		return err
	}
}

// headerize adds the logging headers unless err-file-quiet flag is set
func headerize(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		w := cr.Status.Combined
		start := time.Now()
		if !cr.ErrFileQuiet {
			fmt.Fprintf(w, "// start: %s\n", start.Format(time.RFC3339))
			fmt.Fprintf(w, "// cmd: %s\n", cr.Command)
			if cr.Timeout > 0 {
				fmt.Fprintf(w, "// timeout: %s\n", cr.Timeout)
			}
		}
		err = g(ctx, cr)
		end := time.Now()
		if !cr.ErrFileQuiet {
			fmt.Fprintf(w, "// end: %s\n", end.Format(time.RFC3339))
			fmt.Fprintf(w, "// took: %s\n", end.Sub(start))
			fmt.Fprintf(w, "// exitcode: %d\n", cr.Status.ExitCode)
		}
		if err != nil {
			fmt.Fprintf(w, "// error: %s\n", err.Error())
		}
		return err
	}
}

// lockfile ensures that the cron will only run once if logfile flag is set
func lockfile(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		if cr.Lockfile != "" {
			_, statErr := os.Stat(cr.Lockfile)
			if statErr == nil {
				pidBytes, err := ioutil.ReadFile(cr.Lockfile)
				if err != nil {
					return fmt.Errorf("unable to read lockfile: %s", err)
				}
				pid, err := strconv.Atoi(string(pidBytes))
				if err != nil {
					return fmt.Errorf("unable to read pidfile: %s", err)
				}
				proc, err := os.FindProcess(pid)
				if err != nil {
					return fmt.Errorf("process(%d) from pidfile missing: %s", pid, err)
				}
				err = proc.Signal(syscall.Signal(0))
				if err != nil {
					return fmt.Errorf("process(%d) from pidfile missing: %s", pid, err)
				}
				_, _ = fmt.Fprintf(cr.Status.Combined, "cron is still running, pid: %d", pid)
				return nil
			} else if !os.IsNotExist(statErr) {
				return fmt.Errorf("unable to handle lockfile: %s", statErr)
			}
			pid := os.Getpid()
			lockfile, err := os.OpenFile(cr.Lockfile, os.O_CREATE|os.O_RDWR, 0600)
			if err != nil {
				return fmt.Errorf("unable to open lockfile: %s", err)
			}
			defer lockfile.Close()
			_, err = fmt.Fprintf(lockfile, "%d", pid)
			if err != nil {
				return fmt.Errorf("unable to write lockfile: %s", err)
			}
			defer os.Remove(cr.Lockfile)
		}
		err = g(ctx, cr)
		return err
	}
}

func sentryHandler(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		// check if envar is set
		sentryDSN, ok := os.LookupEnv("CRONGUARD_SENTRY_DSN")
		if !ok && cr.Config != nil {
			sentryDSN = cr.Config.SentryDSN
		}
		if sentryDSN == "" {
			return g(ctx, cr)
		}

		// wrap buffers
		start := time.Now()
		combined := bytes.NewBuffer([]byte{})
		stderr := bytes.NewBuffer([]byte{})
		cr.Status.Stderr = io.MultiWriter(stderr, combined, cr.Status.Stderr)
		cr.Status.Stdout = io.MultiWriter(combined, cr.Status.Stdout)

		// prepare sentry
		sentryErr := sentry.Init(sentry.ClientOptions{
			Dsn:       sentryDSN,
			Transport: sentry.NewHTTPSyncTransport(),
		})
		if sentryErr != nil {
			fmt.Fprintf(cr.Status.Stderr, "cronguard: unable to connect to sentry: %s\n", sentryErr)
			fmt.Fprintf(cr.Status.Stderr, "cronguard: running cron anyways\n")
		}

		err = g(ctx, cr)

		// try to log to sentry
		if err != nil && sentryErr == nil {
			// gather data
			hostname, _ := os.Hostname()
			if hostname == "" {
				hostname = "no-hostname"
			}
			hostname = strings.SplitN(hostname, ".", 2)[0]
			cmd := cr.Command
			if len(cmd) > 32 {
				cmd = fmt.Sprintf("%s%s", cmd[0:30], "...")
			}
			cmdHash := sha256.New()
			cmdHash.Write([]byte(cr.Command))
			cmdHash.Write([]byte(hostname))
			hash := hex.EncodeToString(cmdHash.Sum(nil))

			// add data to message
			sentry.ConfigureScope(func(scope *sentry.Scope) {
				scope.SetExtra("time_start", start)
				scope.SetExtra("time_end", time.Now())
				scope.SetExtra("time_duration", time.Since(start).String())
				scope.SetExtra("out_combined", combined.String())
				scope.SetExtra("out_stderr", stderr.String())
				scope.SetExtra("command", cr.Command)
				scope.SetFingerprint([]string{hash})
			})
			name := fmt.Sprintf(
				"%s: %s (%s)",
				hostname,
				cmd,
				err.Error(),
			)
			_ = sentry.CaptureMessage(name)

			// hide error if messages are successfully flushed to sentry
			flushed := sentry.Flush(30 * time.Second)
			if flushed {
				return nil
			}
		}
		return err
	}

}

// quietIgnore allows to ignore errors on lower settings if flag is set
func quietIgnore(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		quiet, err := isQuiet(cr)
		if err != nil {
			log.Fatalf("quiet-times issue: %s", err)
		}
		err = g(ctx, cr)
		if quiet {
			return nil
		}
		return err
	}

}

// validateStderr requires stderr to be empty
func validateStderr(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		stderr := cr.Status.Stderr
		wc := NewWriteCounter(stderr)
		cr.Status.Stderr = wc
		err = g(ctx, cr)
		if err != nil {
			return err
		}
		if wc.GetCounter() > 0 {
			return errors.New("stderr is not empty")
		}
		return err
	}
}

// validateStdout validates stdout for blacklist regex matches
func validateStdout(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		stdout := cr.Status.Stdout
		out, in := io.Pipe()
		cr.Status.Stdout = io.MultiWriter(stdout, in)

		s := bufio.NewScanner(out)
		errGrp := errgroup.Group{}
		errGrp.Go(func() error {
			var err error
			for s.Scan() {
				line := s.Bytes()
				if readErr := s.Err(); readErr != nil {
					return readErr
				}
				match := cr.Regex.Match(line)
				if match {
					err = fmt.Errorf("bad keyword in command output: %s", line)
				}
			}
			return err
		})

		err = g(ctx, cr)
		if err != nil {
			return
		}
		err = in.Close()
		if err != nil {
			return
		}
		err = errGrp.Wait()
		return err
	}
}

// timeout adds a timeout for the command if flag is set
func timeout(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		ctx, cancel := context.WithCancel(ctx)
		if cr.Timeout > time.Duration(0) {
			ctx, cancel = context.WithTimeout(context.Background(), cr.Timeout)
		}
		defer cancel()
		err = g(ctx, cr)
		if err != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		return err
	}
}
