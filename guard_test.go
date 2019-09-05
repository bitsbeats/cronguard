package main

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type (
	tCase struct {
		command string
		content string
	}

	tQuietCase struct {
		quiet   string
		command string
		content string
	}
)

func guard(t *testing.T, additionalArgs []string, command string, want string) (err error) {
	err = os.Remove("/tmp/goguardtest")
	if err != nil && !os.IsNotExist(err) {
		return
	}

	os.Args = []string{
		"_",
		"-errfile-no-uuid",
		"-errfile-quiet",
		"-name", "test",
		"-errfile", "/tmp/goguardtest",
	}
	os.Args = append(os.Args, additionalArgs...)
	os.Args = append(os.Args, command)
	main()

	got, err := ioutil.ReadFile("/tmp/goguardtest")
	if err != nil {
		return
	}

	if diff := cmp.Diff(want, string(got)); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	err = os.Remove("/tmp/goguardtest")
	if err != nil && !os.IsNotExist(err) {
		return nil
	}

	return
}

func TestOutput(t *testing.T) {
	var err error

	// test normal
	cases := []tCase{
		// check exit statuss
		{"true", ""},
		{"false", "// error: exit status 1\n"},
		{"exit 2", "// error: exit status 2\n"},

		// check output
		{"echo fail", "fail\n// error: bad keyword in command output: fail\n"},
		{"echo failure", "failure\n// error: bad keyword in command output: failure\n"},
		{"echo ERR", "ERR\n// error: bad keyword in command output: ERR\n"},
		{"echo ERROR", "ERROR\n// error: bad keyword in command output: ERROR\n"},
		{"echo Crit", "Crit\n// error: bad keyword in command output: Crit\n"},
		{"echo Critical", "Critical\n// error: bad keyword in command output: Critical\n"},

		// check err output
		{"echo Hi there 1>&2", "Hi there\n// error: stderr is not empty\n"},

		// check asci boundaries
		{"echo transferred", ""},
		{"echo transferred error", "transferred error\n// error: bad keyword in command output: transferred error\n"},
	}
	for _, c := range cases {
		err = guard(t, []string{}, c.command, c.content)
		if err != nil {
			t.Error(err)
			break
		}
	}

	// test with quiet
	qCases := []tQuietCase{
		{"0 * * * *:1h", "false", ""},
		{"0 0 * * *:0s", "false", "// error: exit status 1\n"},
	}


	for _, c := range qCases {
		err = guard(t, []string{"-quiet-times", c.quiet}, c.command, c.content)
		if err != nil {
			t.Error(err)
			break
		}
	}
}
