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

// Add writer to implement io.Writer
func (e *ErrorLogger) Write(p []byte) (n int, err error) {
	if e.showUUID {
		fmt.Fprintf(e.file, "%s == ", e.uuid)
	}
	fmt.Fprintf(e.file, "%s", p)
	return len(p), nil
}
