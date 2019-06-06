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
)

func cron(t *testing.T, command string, want string) (err error) {
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
		command,
	}
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
	cases := []tCase{
		// check exit statuss
		{"true", ""},
		{"false", "errors while running cron.test\nexit status 1\nexit status 1\n"},
		{"exit 2", "errors while running cron.test\nexit status 2\nexit status 2\n"},

		// check output
		{"echo fail", "errors while running cron.test\nfail\nbad keyword in command output\nexit status 0\n"},
		{"echo failure", "errors while running cron.test\nfailure\nbad keyword in command output\nexit status 0\n"},
		{"echo ERR", "errors while running cron.test\nERR\nbad keyword in command output\nexit status 0\n"},
		{"echo ERROR", "errors while running cron.test\nERROR\nbad keyword in command output\nexit status 0\n"},
		{"echo Crit", "errors while running cron.test\nCrit\nbad keyword in command output\nexit status 0\n"},
		{"echo Critical", "errors while running cron.test\nCritical\nbad keyword in command output\nexit status 0\n"},

		// check err output
		{"echo Hi there 1>&2", "errors while running cron.test\nHi there\nstderr is not empty\nexit status 0\n"},
	}

	var err error
	for _, c := range cases {
		err = cron(t, c.command, c.content)
		if err != nil {
			break
		}
	}
}
