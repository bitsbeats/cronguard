package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/syslog"
	"os"
	"time"

	"github.com/rs/zerolog/log"
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
			log.Fatal().Err(errFileErr).Str("file", cr.ErrFile).Msg("error opening")
		}
		defer errFile.Close()

		err = g(ctx, cr)
		log.Debug().Err(err).Msg("executed in setupLogs")

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
			log.Fatal().Err(err).Msg("unable to open syslog")
		}
		defer slog.Close()
		cr.Status.Combined = io.MultiWriter(slog, cr.Status.Combined)
		err = g(ctx, cr)
		log.Debug().Err(err).Msg("executed in writeSyslog")
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
		err = g(ctx, cr)
		log.Debug().Err(err).Msg("executed in insertUUID")
		return err
	}
}

// combinelogs adds stderr and stdout to combined log
func combineLogs(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		combined := NewLockedWriter(cr.Status.Combined)
		cr.Status.Stdout = io.MultiWriter(cr.Status.Stdout, combined)
		cr.Status.Stderr = io.MultiWriter(cr.Status.Stderr, combined)
		err = g(ctx, cr)
		log.Debug().Err(err).Msg("executed in combineLogs")
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
		log.Debug().Err(err).Msg("executed in headerize")

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
			run, err := handleExistingLockfile(cr)
			if err != nil {
				return err
			}
			if !run {
				return nil
			}

			pid := os.Getpid()
			lockfile, err := os.OpenFile(cr.Lockfile, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
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
		log.Debug().Err(err).Msg("executed in lockfile")

		return err
	}
}

// sentryHandler redirects all errors to a sentry if configured
func sentryHandler(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		reporter, reporterErr := newReporter(cr)
		if reporterErr != nil {
			return g(ctx, cr)
		}

		cr.Reporter = reporter

		err = g(ctx, cr)
		log.Debug().Err(err).Msg("executed in sentryHandler")

		return reporter.Finish(err)
	}
}

// quietIgnore allows to ignore errors on lower settings if flag is set
func quietIgnore(g GuardFunc) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		quiet, err := isQuiet(cr)
		if err != nil {
			log.Fatal().Err(err).Msg("quiet-time malformed")
		}

		err = g(ctx, cr)
		log.Debug().Err(err).Msg("executed in quietIgnore")

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
		log.Debug().Err(err).Msg("executed in validateStderr")

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
		log.Debug().Err(err).Msg("executed in validateStdout")

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
		log.Debug().Err(err).Msg("executed in timeout")

		if err != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		return err
	}
}
