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

func guard(t *testing.T, quiet string, command string, want string) (err error) {
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
	if quiet != "" {
		os.Args = append(os.Args, "-quiet-times", quiet)
	}
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
		return
	}

	return
}

func TestOutput(t *testing.T) {
	var err error

	// test normal
	cases := []tCase{
		// check exit statuss
		{"true", ""},
		{"false", "errors while running guard.test\nexit status 1\nexit status 1\n"},
		{"exit 2", "errors while running guard.test\nexit status 2\nexit status 2\n"},

		// check output
		{"echo fail", "errors while running guard.test\nfail\nbad keyword in command output\nexit status 0\n"},
		{"echo failure", "errors while running guard.test\nfailure\nbad keyword in command output\nexit status 0\n"},
		{"echo ERR", "errors while running guard.test\nERR\nbad keyword in command output\nexit status 0\n"},
		{"echo ERROR", "errors while running guard.test\nERROR\nbad keyword in command output\nexit status 0\n"},
		{"echo Crit", "errors while running guard.test\nCrit\nbad keyword in command output\nexit status 0\n"},
		{"echo Critical", "errors while running guard.test\nCritical\nbad keyword in command output\nexit status 0\n"},

		// check err output
		{"echo Hi there 1>&2", "errors while running guard.test\nHi there\nstderr is not empty\nexit status 0\n"},

		// check asci boundaries
		{"echo transferred", ""},
		{"echo transferred error", "errors while running guard.test\ntransferred error\nbad keyword in command output\nexit status 0\n"},
	}
	for _, c := range cases {
		err = guard(t, "", c.command, c.content)
		if err != nil {
			t.Error(err)
			break
		}
	}

	// test with quiet
	qCases := []tQuietCase{
		{"0 * * * *:1h", "false", ""},
		{"0 0 * * *:0s", "false", "errors while running guard.test\nexit status 1\nexit status 1\n"},
	}
	for _, c := range qCases {
		err = guard(t, c.quiet, c.command, c.content)
		if err != nil {
			t.Error(err)
			break
		}
	}
}
