package main

import (
	"fmt"
	"os"

	"github.com/ionut-t/bark/cmd"
	"github.com/ionut-t/coffee/styles"
)

func main() {
	err := cmd.Execute()

	if err != nil {
		fmt.Println(styles.Error.Render("Error: " + err.Error()))
		os.Exit(1)
	}
}
