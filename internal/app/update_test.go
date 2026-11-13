package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/update"
)

type fakeStartupUpdater struct {
	release     *update.Release
	checkErr    error
	installErr  error
	handedOff   bool
	installRuns int
}

func (f *fakeStartupUpdater) Check(context.Context) (*update.Release, error) {
	return f.release, f.checkErr
}

func (f *fakeStartupUpdater) Install(context.Context, update.Release, io.Writer, io.Writer) (bool, error) {
	f.installRuns++
	return f.handedOff, f.installErr
}

func TestStartupUpdateRequiresConfirmationAndStopsAfterInstall(t *testing.T) {
	for _, test := range []struct {
		name, answer string
		installErr   error
		handedOff    bool
		wantStop     bool
		wantInstall  int
	}{
		{name: "default no", answer: "\n"},
		{name: "explicit no", answer: "n\n"},
		{name: "other answer", answer: "maybe\n"},
		{name: "yes", answer: "y\n", wantStop: true, wantInstall: 1},
		{name: "full yes", answer: "YES\n", wantStop: true, wantInstall: 1},
		{name: "failed install", answer: "y\n", installErr: errors.New("permission denied"), wantStop: true, wantInstall: 1},
		{name: "Windows helper", answer: "y\n", handedOff: true, wantStop: true, wantInstall: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output, logs bytes.Buffer
			a := App{Stdin: strings.NewReader(test.answer), Stdout: &output, Stderr: &logs}
			service := &fakeStartupUpdater{release: &update.Release{Tag: "v99.0.0"}, installErr: test.installErr, handedOff: test.handedOff}
			stop, err := a.checkStartupUpdate(context.Background(), service, false)
			if stop != test.wantStop || service.installRuns != test.wantInstall || (err != nil) != (test.installErr != nil) {
				t.Fatalf("stop=%v installs=%d error=%v", stop, service.installRuns, err)
			}
			if !strings.Contains(output.String(), "Update now? [y/N]") {
				t.Fatalf("missing confirmation: %s", output.String())
			}
			wantSuccess := test.wantInstall == 1 && test.installErr == nil && !test.handedOff
			if strings.Contains(output.String(), "Updated SessionMan") != wantSuccess {
				t.Fatalf("incorrect success message: %s", output.String())
			}
		})
	}
}

func TestStartupCheckFailureContinuesWithoutPrompt(t *testing.T) {
	for _, debug := range []bool{false, true} {
		var output, logs bytes.Buffer
		a := App{Stdin: strings.NewReader(""), Stdout: &output, Stderr: &logs}
		service := &fakeStartupUpdater{checkErr: errors.New("original network error")}
		stop, err := a.checkStartupUpdate(context.Background(), service, debug)
		if stop || err != nil || output.Len() != 0 || service.installRuns != 0 {
			t.Fatalf("network failure blocked startup: stop=%v error=%v output=%s", stop, err, &output)
		}
		if strings.Contains(logs.String(), "original network error") != debug {
			t.Fatalf("debug=%v logs=%q", debug, logs.String())
		}
	}
}

func TestStartupUpdateSkipsNonTerminalAndOptOut(t *testing.T) {
	for _, disabled := range []string{"", "1"} {
		t.Setenv("SM_NO_UPDATE_CHECK", disabled)
		var output, logs bytes.Buffer
		a := App{Stdin: strings.NewReader("y\n"), Stdout: &output, Stderr: &logs}
		stop, err := a.CheckStartupUpdate(context.Background(), true)
		if stop || err != nil || output.Len() != 0 || logs.Len() != 0 {
			t.Fatalf("disabled=%q stop=%v error=%v output=%s logs=%s", disabled, stop, err, &output, &logs)
		}
	}
}
