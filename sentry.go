package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	stdlog "log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog/log"
)

type (
	Reporter struct {
		sentryDSN string
		start     time.Time
		hostname  string
		cmd       string
		hash      hash.Hash

		combined *bytes.Buffer
		stderr   *bytes.Buffer
	}
)

var SentryTimeout = 30 * time.Second

// newReporter creates a new Sentry client
func newReporter(cr *CmdRequest) (*Reporter, error) {
	sentryDSN, ok := os.LookupEnv("CRONGUARD_SENTRY_DSN")
	if !ok && cr.Config != nil {
		sentryDSN = cr.Config.SentryDSN
	}
	if sentryDSN == "" {
		return nil, fmt.Errorf("no config provided")
	}

	// data
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "no-hostname"
	}
	hostname = strings.SplitN(hostname, ".", 2)[0]
	hash := sha256.New()
	hash.Write([]byte(cr.Command))
	hash.Write([]byte(hostname))
	cmd := cr.Command
	if len(cmd) > 32 {
		cmd = fmt.Sprintf("%s%s", cmd[0:30], "...")
	}

	// setup sentry
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	sentryErr := sentry.Init(sentry.ClientOptions{
		Debug:         cr.Debug,
		HTTPTransport: transport,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			log.Debug().Interface("event", event).Msg("sending event")
			return event
		},
		Dsn: sentryDSN,
	})
	if sentryErr != nil {
		fmt.Fprintf(cr.Status.Stderr, "cronguard: unable to connect to sentry: %s\n", sentryErr)
		fmt.Fprintf(cr.Status.Stderr, "cronguard: running cron anyways\n")
		return nil, fmt.Errorf("unable to connect to sentry")
	}

	// wrap buffers
	start := time.Now()
	combined := bytes.NewBuffer([]byte{})
	stderr := bytes.NewBuffer([]byte{})
	cr.Status.Stderr = io.MultiWriter(stderr, combined, cr.Status.Stderr)
	cr.Status.Stdout = io.MultiWriter(combined, cr.Status.Stdout)

	// set known sentry extras
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetExtra("time_start", start)
		scope.SetExtra("command", cr.Command)
	})

	if cr.Debug {
		sentry.Logger = stdlog.Default()
	}

	return &Reporter{
		sentryDSN: sentryDSN,
		start:     start,
		hostname:  hostname,
		cmd:       cmd,
		hash:      hash,
		combined:  combined,
		stderr:    stderr,
	}, nil
}

// Finish reports the final status to sentry if err != nil
func (r *Reporter) Finish(err error) error {
	if err == nil {
		return nil
	}
	return r.report(err, finishLevel)
}

// Info reports a Info status to sentry
func (r *Reporter) Info(err error) error {
	return r.report(err, infoLevel)
}

// reportLevel is used by reporter to disingques
type reportLevel = string

const (
	// infoLevel is an information that will be send to sentry
	infoLevel reportLevel = "info"
	// finishLevel is used to tell the reporter that the cron has finished
	finishLevel = "finish"
)

// report reports any error message to sentry
func (r *Reporter) report(err error, level reportLevel) error {
	// prepare sentry information
	name := ""
	extra := map[string]interface{}{}
	if level == finishLevel {
		name = fmt.Sprintf("%s: %s (%s)", r.hostname, r.cmd, err.Error())
		extra["time_end"] = time.Now()
		extra["time_duration"] = time.Since(r.start).String()
		extra["out_combined"] = r.combined.String()
		extra["out_stderr"] = r.stderr.String()
	} else {
		name = fmt.Sprintf("%s (%s): %s (%s)", r.hostname, level, r.cmd, err.Error())
	}

	// sentry
	hash := hex.EncodeToString(r.hash.Sum([]byte(level)))
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetFingerprint([]string{hash})
		scope.SetExtras(extra)
	})
	_ = sentry.CaptureMessage(name)

	// hide error if messages are successfully flushed to sentry
	flushed := sentry.Flush(SentryTimeout)
	if !flushed {
		return err
	}
	return nil
}
