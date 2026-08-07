package main

import (
	"os"

	"github.com/nickmisasi/cursor-utils/cursorctl/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
