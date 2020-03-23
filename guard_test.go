package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

type (
	tCase struct {
		command        string
		additionalArgs []string
		content        string
	}
)

func guard(t *testing.T, additionalArgs []string, command string, want string) (err error) {
	fmt.Printf("running %s\n", command)
	tempFile, err := ioutil.TempFile("", "guard")
	if err != nil {
		return fmt.Errorf("unable to create tmp file (%s): %s", tempFile.Name(), err)
	}

	os.Args = []string{
		"_",
		"-errfile-no-uuid",
		"-errfile-quiet",
		"-name", "test",
		"-errfile", tempFile.Name(),
	}
	os.Args = append(os.Args, additionalArgs...)
	os.Args = append(os.Args, command)
	main()

	got, err := ioutil.ReadAll(tempFile)
	if err != nil {
		return
	}

	if strings.ContainsRune(string(got), '\x00') {
		return fmt.Errorf("Nullbyte")
	}
	if diff := cmp.Diff(want, string(got)); diff != "" {
		return fmt.Errorf("mismatch (-want +got):\n%s", diff)
	}

	tempFile.Close()
	err = os.Remove(tempFile.Name())
	if err != nil && !os.IsNotExist(err) {
		return nil
	}

	return err
}

func TestOutput(t *testing.T) {
	var err error

	// test normal
	cases := []tCase{
		// check exit statuss
		{"true", []string{}, ""},
		{"false", []string{}, "// error: exit status 1\n"},
		{"exit 2", []string{}, "// error: exit status 2\n"},

		// check output
		{"echo fail", []string{}, "fail\n// error: bad keyword in command output: fail\n"},
		{"echo failure", []string{}, "failure\n// error: bad keyword in command output: failure\n"},
		{"echo ERR", []string{}, "ERR\n// error: bad keyword in command output: ERR\n"},
		{"echo ERROR", []string{}, "ERROR\n// error: bad keyword in command output: ERROR\n"},
		{"echo Crit", []string{}, "Crit\n// error: bad keyword in command output: Crit\n"},
		{"echo Critical", []string{}, "Critical\n// error: bad keyword in command output: Critical\n"},
		{`echo -e "err\ngood line\n"`, []string{}, "err\ngood line\n\n// error: bad keyword in command output: err\n"},

		// check err output
		{"echo Hi there 1>&2", []string{}, "Hi there\n// error: stderr is not empty\n"},

		// check asci boundaries
		{"echo transferred", []string{}, ""},
		{"echo transferred error", []string{}, "transferred error\n// error: bad keyword in command output: transferred error\n"},

		// quiet tests
		{"false", []string{"-quiet-times", "0 * * * *:1h"}, ""},
		{"false", []string{"-quiet-times", "0 0 * * *:0s"}, "// error: exit status 1\n"},

		// timeout tests
		{"sleep 1", []string{"-timeout", "2s"}, ""},
		{"sleep 2", []string{"-timeout", "500ms"}, "// error: context deadline exceeded\n"},
	}
	for _, c := range cases {
		err = guard(t, c.additionalArgs, c.command, c.content)
		if err != nil {
			t.Error(err)
			break
		}
	}

	// paralell tests
	cases = []tCase{
		// lockfile tests
		{"sleep 2; echo failed", []string{"-lockfile", "/tmp/guard.lock"}, "failed\n// error: bad keyword in command output: failed\n"},
		{"echo failed this should not run; sleep 3", []string{"-lockfile", "/tmp/guard.lock"}, ""},
	}
	errChan := make(chan error)
	for _, c := range cases {
		<-time.After(1 * time.Second)
		go func(c tCase) {
			err := guard(t, c.additionalArgs, c.command, c.content)
			errChan <- err
		}(c)
	}
	for i := 0; i < len(cases); i++ {
		err = <-errChan
		if err != nil {
			t.Error(err)
		}
	}
}
