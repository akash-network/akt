package main

import (
	"github.com/akash-network/akt/logo"
	"os"
)

func main() {
	logo.Write(os.Stdout)
	showLoading()
}
