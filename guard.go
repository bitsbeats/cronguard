package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"time"
)

type (
	// CmdRequest ist a guarded command request
	CmdRequest struct {
		Name    string
		Command string

		ErrFile         string
		ErrFileQuiet    bool
		ErrFileHideUUID bool

		QuietTimes string
		Timeout    time.Duration
		Lockfile   string

		Regex *regexp.Regexp

		Config *Config

		Status *CmdStatus
	}

	// CmdStatus is the commands status
	CmdStatus struct {
		Stdout   io.Writer // captures stdout
		Stderr   io.Writer // captures stderr
		Combined io.Writer // captures stdout and stderr
		ExitCode int       // captures the exitcode
	}

	// GuardFunc is a middleware function
	GuardFunc func(ctx context.Context, cr *CmdRequest) (err error)
)

func main() {
	cr := CmdRequest{}
	cr.Config = ParseConfig()
	cr.Status = &CmdStatus{}
	f := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	f.StringVar(&cr.Name, "name", "guard", "cron name in syslog")
	f.StringVar(&cr.ErrFile, "errfile", "/var/log/cronstatus", "error report file")
	f.BoolVar(&cr.ErrFileQuiet, "errfile-quiet", false, "hide timings in error report file")
	f.BoolVar(&cr.ErrFileHideUUID, "errfile-no-uuid", false, "hide uuid in error report file")
	f.StringVar(&cr.QuietTimes, "quiet-times", "", "time ranges to ignore errors, format 'start(cron format):duration(golang duration):...")
	f.DurationVar(&cr.Timeout, "timeout", 0, "timeout for the cron, set to enable")
	f.StringVar(&cr.Lockfile, "lockfile", "", "lockfile to prevent the cron running twice, set to enable")
	regexFlag := f.String("regex", `(?im)\b(err|fail|crit)`, "regex for bad words")
	if err := f.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	cr.Regex = regexp.MustCompile(*regexFlag)
	if len(f.Args()) != 1 {
		log.Fatalf("more than one command argument given: '%v'", f.Args())
	}
	cr.Command = f.Arg(0)

	r := chained(
		runner, timeout, validateStdout, validateStderr, quietIgnore,
		sentryHandler, lockfile, headerize, combineLogs, insertUUID,
		writeSyslog, setupLogs,
	)
	err := r(context.Background(), &cr)
	if err != nil {
		log.Fatalf("execution failed: %s", err)
	}
}

// chained chaines all the middlewares together (reversed execution order)
func chained(final func() GuardFunc, middlewares ...func(GuardFunc) GuardFunc) (g GuardFunc) {
	g = final()
	for _, m := range middlewares {
		g = m(g)
	}
	return
}

// runner executes the guarded command
func runner() GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) (err error) {
		cmd := exec.CommandContext(ctx, "bash", "-c", cr.Command)
		cmd.Stdout = cr.Status.Stdout
		cmd.Stderr = cr.Status.Stderr

		err = cmd.Start()
		if err != nil {
			return fmt.Errorf("unable to run command: %s", err)
		}

		err = cmd.Wait()
		if err != nil {
			switch casted := err.(type) {
			case *exec.ExitError:
				cr.Status.ExitCode = casted.ExitCode()
			default:
				cr.Status.ExitCode = 1
				err = fmt.Errorf("unable to execute command: %w", err)
			}
			return err
		}
		return err
	}

}
