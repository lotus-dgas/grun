package main

import (
	"flag"
	"fmt"
)

func getCmdLineArg() {
	var defaultForks int
	if cfg.Forks == 0 {
		defaultForks = 100
	} else {
		defaultForks = cfg.Forks
	}

	forks := flag.Int("f", defaultForks, "set concurrent num")
	debug := flag.Bool("d", false, "open debug mode")
	notColorPrint := flag.Bool("nc", false, "close color print")
	notBackOnCopy := flag.Bool("nb", false, "close backup when copy")

	become := flag.Bool("b", false, "if run cmd as root")
	remoteRun := flag.Bool("r", false, "copy script file to remote and run")
	noNewline := flag.Bool("n", false, "print result without new line between ip and result")
	copy := flag.Bool("c", false, "only copy local file to remote machine's some directory[can config]")
	server = flag.Bool("server", false, "open server mode")
	client = flag.Bool("client", false, "open client mod")
	flag.Parse()
	other := flag.Args()

	if len(other) != 0 {
		cmd = other[0]
	}
	cfg.Forks = *forks
	if *remoteRun != false {
		cfg.RemoteRun = *remoteRun
	}
	if *noNewline != false {
		cfg.AddNewline = false
	}
	if *copy != false {
		cfg.Copy = *copy
	}
	if *debug != false {
		cfg.Debug = *debug
	}
	if *become != false {
		cfg.Become = *become
	}
	if *notColorPrint != false {
		cfg.ColorPrint = false
	}
	if *notBackOnCopy != false {
		cfg.BackOnCopy = false
	}
	if cfg.Debug {
		fmt.Printf("[debug]%+v", cfg)
	}
}