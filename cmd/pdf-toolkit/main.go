// Command pdf-toolkit merges, splits, reorders, rotates, compresses, and
// password-protects PDFs entirely locally — pdfcpu is compiled directly
// into this binary, so there's nothing to install and nothing uploaded
// anywhere. Bare invocation opens a local browser UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/DavidMarsanic/pdf-toolkit/internal/browser"
	"github.com/DavidMarsanic/pdf-toolkit/internal/paths"
	"github.com/DavidMarsanic/pdf-toolkit/internal/server"
)

const version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("pdf-toolkit", flag.ContinueOnError)

	output := fs.String("output", "", "output directory (default: your Downloads folder)")
	port := fs.Int("port", 0, "local UI server port (default: automatic)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Usage = func() { printUsage(fs) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if *showVersion {
		fmt.Println("pdf-toolkit " + version)
		return 0
	}

	outputDir, err := paths.ResolveDownloadsDir(*output)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New(ctx, outputDir)
	addr, err := srv.Start(*port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	fmt.Fprintln(os.Stderr, "PDF Toolkit running at", addr, "— press Ctrl+C to quit")

	// When a host process (securexe-launcher) is the one showing the UI —
	// in its own native window, so it can get a real Dock identity instead
	// of a spawned Chrome window — it sets this before starting us and
	// watches this same stderr line to discover the URL. Opening our own
	// Chrome window too would just leave a second, redundant one.
	if os.Getenv("SECUREXE_HOSTED") == "" {
		if err := browser.OpenAppWindow(addr + "/"); err != nil {
			fmt.Fprintln(os.Stderr, "couldn't open a window automatically:", err)
			fmt.Fprintln(os.Stderr, "open this URL manually:", addr+"/")
		}
	}

	<-ctx.Done()
	return 0
}

func printUsage(fs *flag.FlagSet) {
	fmt.Fprint(os.Stderr, `pdf-toolkit — merge, split, reorder, rotate, compress, and
password-protect PDFs, entirely on this machine.

Bare invocation opens a local browser UI: drop a PDF (or several), work on
its pages, and export. Results are saved to your Downloads folder.

Usage:
  pdf-toolkit                open the browser UI

Flags:
`)
	fs.PrintDefaults()
}
