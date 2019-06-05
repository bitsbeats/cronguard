# Cronguard

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
