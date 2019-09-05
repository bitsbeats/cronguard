# Cronguard

[![Go Report Card](https://goreportcard.com/badge/github.com/bitsbeats/cronguard)](https://goreportcard.com/report/github.com/bitsbeats/cronguard)
[![Build Status](https://cloud.drone.io/api/badges/bitsbeats/cronguard/status.svg)](https://cloud.drone.io/bitsbeats/cronguard)

Simple wrapper log and handle cron errors.

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

```
cronguard -name cron.example "command"
```

The command is executed with `bash -c`. You can use bash features like pipes.

**Note**: Bash is required.

### Quiet-Times

Using quiet-times you can setup time-ranges where errors are ignores. Useful if there is a database backup and you want to disable to errors during the backup.

Example:

```sh
cronguard -quiet-times "0 2 * * *:42m:0 5 * * *:20s" "echo hello world"
```

Here we ignore errors starting at 2:00 for 42minutes and starting at 05:00 for 20 seconds.

Cron format documentation: https://godoc.org/github.com/robfig/cron#hdr-CRON_Expression_Format  
Golang time duration documentation: https://golang.org/pkg/time/#ParseDuration

## Install

Via go:

```
go get -u github.com/bitsbeats/cronguard
```
