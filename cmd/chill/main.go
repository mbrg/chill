package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "Usage: chill <command>")
	os.Exit(1)
}
