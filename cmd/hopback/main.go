// Command hopback keeps track of where your projects stand across AI coding agent sessions.
package main

import (
	"os"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/cli"
)

func main() { os.Exit(cli.Execute()) }
