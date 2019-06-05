# Cronguard

[![Go Report Card](https://goreportcard.com/badge/github.com/bitsbeats/cronguard)](https://goreportcard.com/report/github.com/bitsbeats/cronguard)
[![Build Status](https://cloud.drone.io/api/badges/bitsbeats/cronguard/status.svg)](https://cloud.drone.io/bitsbeats/cronguard)

Simple wrapper log and handle cron errors.

# Usage

```
  -errfile string
    	error report file (default "/var/log/cronstatus")
  -name string
    	cron name in syslog (default "general")
  -no-err-uuid
    	hide uuid in error report file
```

Example:

```
cronguard -name cron.example "command"
```

The command is executed with `bash -c`. You can use bash features like pipes.

# Install

Via go:

```
go get -u github.com/bitsbeats/cronguard
```
