package errlog

import (
	"fmt"
	"io"
)

type (
	// ErrorLogger for printing messages to the errorlog
	ErrorLogger struct {
		showUUID bool
		uuid     string
		file     io.Writer
	}
)

// New ErrorLogger instance
func New(showUUID bool, uuid string, file io.Writer) *ErrorLogger {
	return &ErrorLogger{showUUID, uuid, file}
}

// Printf messages to the errorlog
func (e *ErrorLogger) Printf(msg string, fields ...interface{}) {
	if e.showUUID {
		msg = fmt.Sprintf("%s == %s", e.uuid, msg)
	}
	fmt.Fprintf(e.file, msg, fields...)
}
