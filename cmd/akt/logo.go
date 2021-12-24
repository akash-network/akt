package main

import (
	"time"

	_ "embed"

	"github.com/briandowns/spinner"
)

func showLoading() {
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Prefix = "    "
	s.Suffix = "  NEW CLI LOADING..."
	s.Start()
	time.Sleep(20 * time.Second)
	s.Stop()
}
