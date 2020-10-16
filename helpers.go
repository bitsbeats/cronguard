package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/robfig/cron"
	"github.com/rs/xid"
)

// uuidPrefixer is a io.Writer that prefixes every line with UUID
type uuidPrefixer struct {
	uuid          []byte
	writer        io.Writer
	buf           *bytes.Buffer
	lastIsNewline bool
}

// newUUIDPrefixer generates a new uuidPrefixer with an UUID
func newUUIDPrefixer(dest io.Writer) *uuidPrefixer {
	return &uuidPrefixer{
		uuid:          []byte(xid.New().String() + " "),
		writer:        dest,
		buf:           bytes.NewBuffer(nil),
		lastIsNewline: true,
	}
}

// Write satisfies golang io.Writer
func (prefixer *uuidPrefixer) Write(p []byte) (int, error) {
	prefixer.buf.Reset()
	for _, b := range p {
		if prefixer.lastIsNewline {
			prefixer.buf.Write(prefixer.uuid)
			prefixer.lastIsNewline = false
		}
		prefixer.buf.WriteByte(b)
		if b == '\n' {
			prefixer.lastIsNewline = true
		}
	}
	n, err := prefixer.writer.Write(prefixer.buf.Bytes())
	if n > len(p) {
		n = len(p)
	}
	return n, err
}

func isQuiet(cr *CmdRequest) (bool, error) {
	if cr.QuietTimes == "" {
		return false, nil
	}
	ts := strings.Split(cr.QuietTimes, ":")
	if len(ts)%2 != 0 {
		return false, errors.New("invalid quiet-times format")
	}

	for i := 0; i < len(ts); i += 2 {
		startStr := ts[i]
		durStr := ts[i+1]
		now := time.Now()
		shed, err := cron.Parse(startStr)
		if err != nil {
			return false, fmt.Errorf("unable to parse cron time: %s", err)
		}
		dur, err := time.ParseDuration(durStr)
		if err != nil {
			return false, fmt.Errorf("unable to parse duration: %s", err)
		}
		start := shed.Next(now.Add(-dur))
		end := start.Add(dur)
		if now.After(start) && end.After(now) {
			return true, nil
		}
	}
	return false, nil
}
