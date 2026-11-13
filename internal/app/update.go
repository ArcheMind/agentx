package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ArcheMind/agentx/internal/update"
	"golang.org/x/term"
)

type startupUpdater interface {
	Check(context.Context) (*update.Release, error)
	Install(context.Context, update.Release, io.Writer, io.Writer) (bool, error)
}

func (a App) CheckStartupUpdate(ctx context.Context, debug bool) (bool, error) {
	if os.Getenv("SM_NO_UPDATE_CHECK") == "1" {
		return false, nil
	}
	input, inputOK := a.Stdin.(*os.File)
	output, outputOK := a.Stdout.(*os.File)
	if !inputOK || !outputOK || !term.IsTerminal(int(input.Fd())) || !term.IsTerminal(int(output.Fd())) {
		return false, nil
	}
	return a.checkStartupUpdate(ctx, update.New(Version, debug, a.Stderr), debug)
}

func (a App) checkStartupUpdate(ctx context.Context, service startupUpdater, debug bool) (bool, error) {
	release, err := service.Check(ctx)
	if err != nil {
		if debug {
			fmt.Fprintf(a.Stderr, "update check failed: %v\n", err)
		}
		return false, nil
	}
	if release == nil {
		return false, nil
	}
	fmt.Fprintf(a.Stdout, "SessionMan update available: v%s → %s\n", strings.TrimPrefix(Version, "v"), release.Tag)
	answer, err := a.readLine(bufio.NewReader(a.Stdin), "Update now? [y/N] ")
	if err != nil {
		return true, err
	}
	if answer = strings.ToLower(strings.TrimSpace(answer)); answer != "y" && answer != "yes" {
		return false, nil
	}
	fmt.Fprintln(a.Stdout, "Downloading and verifying update...")
	handedOff, err := service.Install(ctx, *release, a.Stdout, a.Stderr)
	if err != nil {
		return true, fmt.Errorf("update failed: %w", err)
	}
	if !handedOff {
		fmt.Fprintf(a.Stdout, "Updated SessionMan to %s.\n", release.Tag)
	}
	return true, nil
}
