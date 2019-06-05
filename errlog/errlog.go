package errlog

import (
	"fmt"
	"io"
)

type (
	Elog struct {
		showUUID bool
		uuid     string
		file     io.Writer
	}
)

func New(showUUID bool, uuid string, file io.Writer) *Elog {
	return &Elog{showUUID, uuid, file}
}

func (e *Elog) Printf(msg string, fields ...interface{}) {
	if e.showUUID {
		msg = fmt.Sprintf("%s == %s", e.uuid, msg)
	}
	fmt.Fprintf(e.file, msg, fields...)
}
