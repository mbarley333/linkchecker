package main

import (
	"linkchecker"
	"os"
)

func main() {
	err := linkchecker.RunCLI()
	if err != nil {
		os.Exit(1)
	}

}
