package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"net/http"
	"os"
	"regexp"
	"testing"
	"encoding/json"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type Suite struct{}

var _ = check.Suite(&Suite{})

func (s *Suite) TestSetupLogs(c *check.C) {
	mockCases := newMockCases()
	// setupLogs consumes stdout
	mockCases[0].validate(c, setupLogs, mockStdout(""), mockCombined(""))
	// setupLogs consumes stdout
	mockCases[1].validate(c, setupLogs, mockStdout(""), mockCombined(""))
	// setupLogs consumes stderr
	mockCases[2].validate(c, setupLogs, mockStderr(""), mockCombined(""))
	// setupLogs consumes errors
	mockCases[3].validate(c, setupLogs, mockNoError{})
}

func (s *Suite) TestWriteSyslog(c *check.C) {
	mockCases := newMockCases()
	for _, cse := range mockCases {
		cse.validate(c, writeSyslog)
	}
}

func (s *Suite) TestInsertUUID(c *check.C) {
	mockCases := newMockCases()
	mockCases[0].validate(c, insertUUID, mockCombinedCheck(&check.Matches), mockCombined(".................... I am happy and ok\n"))
	mockCases[1].validate(c, insertUUID, mockCombinedCheck(&check.Matches), mockCombined(".................... ERR: something went wrong\n"))
	mockCases[2].validate(c, insertUUID, mockCombinedCheck(&check.Matches), mockCombined(".................... oops\n"))
	for _, cse := range mockCases[3:4] {
		c.Logf("test: %+v", cse)
		cse.validate(c, insertUUID)
	}
}

func (s *Suite) TestCombineLogs(c *check.C) {
	// no cases since its already always inlined
}

func (s *Suite) TestHeaderize(c *check.C) {
	mockCases := newMockCases()

	base := "(?s)// start.*%s.*exitcode: 0\n"

	mockCases[0].validate(c, headerize, mockCombinedCheck(&check.Matches), mockCombined(fmt.Sprintf(base, "I am happy and ok")))
	mockCases[1].validate(c, headerize, mockCombinedCheck(&check.Matches), mockCombined(fmt.Sprintf(base, "ERR: something went wrong")))
	mockCases[2].validate(c, headerize, mockCombinedCheck(&check.Matches), mockCombined(fmt.Sprintf(base, "oops")))
	mockCases[3].validate(c, headerize, mockCombinedCheck(&check.Matches), mockCombined("(?s).*exitcode: 1\n$"))
	mockCases[4].validate(c, headerize, mockCombinedCheck(&check.Matches), mockCombined("(?s).*error: problems\n$"))
}

func (s *Suite) TestSentryHandler(c *check.C) {
	// disabled
	mockCases := newMockCases()
	for _, cse := range mockCases {
		cse.validate(c, sentryHandler)
	}

	// http testserver
	mux := http.NewServeMux()
	mux.HandleFunc("/api/1/store/", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)
		message, ok := payload["message"].(string)
		c.Assert(ok, check.Equals, true)
		c.Assert(message, check.Matches, `.*: echo running go tests \(problems\)`)
		extra, ok := payload["extra"].(map[string]interface{})
		c.Assert(ok, check.Equals, true)
		command, ok := extra["command"].(string)
		c.Assert(ok, check.Equals, true)
		c.Assert(command, check.Equals, "echo running go tests")
		out_combined, ok := extra["out_combined"].(string)
		c.Assert(ok, check.Equals, true)
		c.Assert(out_combined, check.Equals, "hi")
	})
	server := httptest.NewServer(mux)

	// enabled working
	os.Setenv("CRONGUARD_SENTRY_DSN", fmt.Sprintf("http://testuser@%s/1", server.Listener.Addr().String()))
	mockCases = newMockCases()
	for _, cse := range mockCases[0:3] {
		cse.validate(c, sentryHandler)
	}
	mockCases[4].validate(c, sentryHandler, mockNoError{})

	// enabled broken
	os.Setenv("CRONGUARD_SENTRY_DSN", fmt.Sprintf("http://%s/2", server.Listener.Addr().String()))
	mockCases = newMockCases()
	for _, cse := range mockCases {
		cse.validate(c, sentryHandler, mockStderrCheck(&check.Matches), mockStderr("(?s).*empty username.*"))
	}
}

func (s *Suite) TestQuietIgnore(c *check.C) {
	mockCases := newMockCases()
	for _, cse := range mockCases {
		cse.validate(c, quietIgnore)
	}
}

func (s *Suite) TestValidateStderr(c *check.C) {
	mockCases := newMockCases()
	mockCases[0].validate(c, validateStderr)
	mockCases[1].validate(c, validateStderr)
	mockCases[2].validate(c, validateStderr, mockError(fmt.Errorf("stderr is not empty")))
	mockCases[3].validate(c, validateStderr)
	mockCases[4].validate(c, validateStderr)
}

func (s *Suite) TestValidateStdout(c *check.C) {
	mockCases := newMockCases()
	mockCases[0].validate(c, validateStdout)
	mockCases[1].validate(c, validateStdout, mockError(fmt.Errorf("bad keyword in command output: ERR: something went wrong")))
	mockCases[2].validate(c, validateStdout)
	mockCases[3].validate(c, validateStdout)
	mockCases[4].validate(c, validateStdout)
}

func (s *Suite) TestTimeout(c *check.C) {
	mockCases := newMockCases()
	for _, cse := range mockCases {
		cse.validate(c, timeout)
	}
}

type (
	mockStdout   string
	mockStderr   string
	mockCombined string
	mockExitcode int
	mockError    error
	mockNoError  struct{}

	mockStdoutCheck   *check.Checker
	mockStderrCheck   *check.Checker
	mockCombinedCheck *check.Checker
	mockExitcodeCheck *check.Checker
	mockErrorCheck    *check.Checker

	mockCase struct {
		cr              *CmdRequest
		name            string
		f               GuardFunc
		defaultStdout   string
		defaultStderr   string
		defaultCombined string
		defaultExitcode int
		defaultError    error
	}
)

func noop() GuardFunc {
	return func(context.Context, *CmdRequest) error {
		return nil
	}
}

func (mc mockCase) validate(c *check.C, gf func(GuardFunc) GuardFunc, overwrites ...interface{}) {
	f, _ := ioutil.TempFile("", "")
	defer f.Close()
	defer os.Remove(f.Name())

	cr := &CmdRequest{}
	cr.Command = "echo running go tests"
	cr.ErrFile = f.Name()
	cr.Status = &CmdStatus{}
	cr.Regex = regexp.MustCompile(`(?im)\b(err|fail|crit)`)
	combined := bytes.NewBuffer([]byte{})
	cr.Status.Combined = combined
	stdout := bytes.NewBuffer([]byte{})
	cr.Status.Stdout = stdout
	stderr := bytes.NewBuffer([]byte{})
	cr.Status.Stderr = stderr

	err := gf(combineLogs(mc.f))(context.Background(), cr)

	shouldStdout := mc.defaultStdout
	shouldStderr := mc.defaultStderr
	shouldCombined := mc.defaultCombined
	shouldExitcode := mc.defaultExitcode
	shouldError := mc.defaultError

	stdoutCheck := check.DeepEquals
	stderrCheck := check.DeepEquals
	combinedCheck := check.DeepEquals
	exitcodeCheck := check.DeepEquals
	errorCheck := check.DeepEquals
	for _, overwrite := range overwrites {
		switch casted := overwrite.(type) {
		case mockStdout:
			shouldStdout = string(casted)
		case mockStderr:
			shouldStderr = string(casted)
		case mockCombined:
			shouldCombined = string(casted)
		case mockExitcode:
			shouldExitcode = int(casted)
		case mockError:
			shouldError = error(casted)
		case mockNoError:
			shouldError = nil
		case mockStdoutCheck:
			stdoutCheck = *casted
		case mockStderrCheck:
			stderrCheck = *casted
		case mockCombinedCheck:
			combinedCheck = *casted
		case mockExitcodeCheck:
			exitcodeCheck = *casted
		case mockErrorCheck:
			errorCheck = *casted
		default:
			c.Fatalf("unsupported overwrite: %T %+v", overwrite, overwrite)
		}

	}

	c.Assert(stdout.String(), stdoutCheck, shouldStdout)
	c.Assert(stderr.String(), stderrCheck, shouldStderr)
	c.Assert(combined.String(), combinedCheck, shouldCombined)
	c.Assert(cr.Status.ExitCode, exitcodeCheck, shouldExitcode)
	c.Assert(err, errorCheck, shouldError)
}

// mockCases are default tests that expect the result is the same as the input
// NOTE: a GuardFunc that implements fail/nonfail logic should have testcases adapted to its logic
func newMockCases() []mockCase {
	return []mockCase{
		{
			name:            "everything ok",
			f:               mockRunner("I am happy and ok\n", "", 0, nil),
			defaultStdout:   "I am happy and ok\n",
			defaultStderr:   "",
			defaultCombined: "I am happy and ok\n",
			defaultExitcode: 0,
			defaultError:    nil,
		},
		{
			name:            "bad keyword",
			f:               mockRunner("ERR: something went wrong\n", "", 0, nil),
			defaultStdout:   "ERR: something went wrong\n",
			defaultStderr:   "",
			defaultCombined: "ERR: something went wrong\n",
			defaultExitcode: 0,
			defaultError:    nil,
		},
		{
			name:            "stderr not empty",
			f:               mockRunner("", "oops\n", 0, nil),
			defaultStdout:   "",
			defaultStderr:   "oops\n",
			defaultCombined: "oops\n",
			defaultExitcode: 0,
			defaultError:    nil,
		},
		{
			name:            "exitcode node zero",
			f:               mockRunner("", "", 1, nil),
			defaultStdout:   "",
			defaultStderr:   "",
			defaultCombined: "",
			defaultExitcode: 1,
			defaultError:    nil,
		},
		{
			name:            "an error was returned",
			f:               mockRunner("hi", "", 0, fmt.Errorf("problems")),
			defaultStdout:   "hi",
			defaultStderr:   "",
			defaultCombined: "hi",
			defaultExitcode: 0,
			defaultError:    fmt.Errorf("problems"),
		},
	}
}

func mockRunner(stdout, stderr string, exitcode int, mockErr error) GuardFunc {
	return func(ctx context.Context, cr *CmdRequest) error {
		err := error(nil)
		_, err = io.WriteString(cr.Status.Stdout, stdout)
		if err != nil {
			return err
		}
		_, err = io.WriteString(cr.Status.Stderr, stderr)
		if err != nil {
			return err
		}

		cr.Status.ExitCode = exitcode
		return mockErr
	}

}
