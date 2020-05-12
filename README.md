# Cronguard

[![Go Report Card](https://goreportcard.com/badge/github.com/bitsbeats/cronguard)](https://goreportcard.com/report/github.com/bitsbeats/cronguard)
[![Build Status](https://cloud.drone.io/api/badges/bitsbeats/cronguard/status.svg)](https://cloud.drone.io/bitsbeats/cronguard)

Simple wrapper to log and handle cron errors.

## Usage

```
  -errfile string
    	error report file (default "/var/log/cronstatus")
  -errfile-no-uuid
    	hide uuid in error report file
  -errfile-quiet
    	hide timings in error report file
  -lockfile string
    	lockfile to prevent the cron running twice, set to enable
  -name string
    	cron name in syslog (default "cron")
  -quiet-times string
    	time ranges to ignore errors, format 'start(cron format):duration(golang duration):...
  -regex string
    	regex for bad words (default "(?im)\\b(err|fail|crit)")
  -timeout duration
    	timeout for the cron, set to enable
```

Example:

```sh
cronguard -name cron.example "command"
```

The command is executed with `bash -c`. You can use bash features like pipes.

**Note**: Bash is required.

### Sentry Support

To enable sentry you can either create a `/etc/cronguard.yaml`, create `./cronguard.yaml` or use the environment
variable `CRONGUARD_SENTRY_DSN`.

If one of these is set cronguard will try to send events to sentry. If thats not possible it will fallback to default
behavior.

Config Example:

```yaml
sentry_dsn: https://00000000000000000000000000000000@sentry.example.com/2
```

### Quiet-Times

Using `-quiet-times` one can setup time ranges during which errors are ignored. Useful to disable error handling,
for example, if there is a database backup running.

Example:

```sh
cronguard -quiet-times "0 2 * * *:42m:0 5 * * *:20s" "echo hello world"
```

This ignores errors starting at 2:00 for 42minutes and starting at 05:00 for 20 seconds.

Cron format documentation: https://godoc.org/github.com/robfig/cron#hdr-CRON_Expression_Format  
Golang time duration documentation: https://golang.org/pkg/time/#ParseDuration

## Install

Via go:

```sh
go get -u github.com/bitsbeats/cronguard
```
