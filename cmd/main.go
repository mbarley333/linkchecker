package main

import (
	"fmt"
	"linkchecker"
	"os"
)

func main() {
	err := linkchecker.RunCLI()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

}
