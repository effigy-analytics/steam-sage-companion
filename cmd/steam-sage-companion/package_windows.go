package main

import (
	"flag"
	"fmt"
	"github.com/effigy-analytics/steam-sage-companion/internal/install"
	"io"
	"os"
)

func packageCommand(command string, args []string) bool {
	if command != "package-check" {
		return false
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	operation := flags.String("operation", "", "package inspection operation")
	stage := flags.String("stage-dir", "", "private staging directory")
	report := flags.String("report", "", "private report file")
	err := flags.Parse(args)
	if err == nil && flags.NArg() != 0 {
		err = fmt.Errorf("unexpected package inspection argument")
	}
	if err == nil {
		err = install.InspectPackage(*operation, *stage, *report)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return true
}
