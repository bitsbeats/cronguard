package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/robfig/cron"
	"golang.org/x/sync/errgroup"
)

func uuidPrefix(dest io.Writer, errGrp *errgroup.Group, UUID []byte) io.WriteCloser {
	out, in := io.Pipe()
	s := bufio.NewScanner(out)
	errGrp.Go(func() (err error) {
		for s.Scan() {
			line := s.Bytes()
			log := make([]byte, len(UUID)+len(line)+2)
			pos := 0
			for _, b := range [][]byte{UUID, []byte(" "), line, []byte("\n")} {
				pos += copy(log[pos:], b)
			}
			_, err = dest.Write(log)
			if err != nil {
				return err
			}
		}
		return
	})
	return in
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
			return false, fmt.Errorf("Unable to parse cron time: %s", err)
		}
		dur, err := time.ParseDuration(durStr)
		if err != nil {
			return false, fmt.Errorf("Unable to parse duration: %s", err)
		}
		start := shed.Next(now.Add(-dur))
		end := start.Add(dur)
		if now.After(start) && end.After(now) {
			return true, nil
		}
	}
	return false, nil
}
