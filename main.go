package main

import (
	"os"

	"github.com/ionut-t/bark/cmd"
)

func main() {
	err := cmd.Execute()
	if err != nil {
		cmd.PrintError(err)
		os.Exit(1)
	}
}
