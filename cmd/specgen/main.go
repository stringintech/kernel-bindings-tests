package main

import (
	"fmt"
	"os"
)

func main() {
	if err := GenerateMethodReference("docs/schemas", "docs/methods-spec.md"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
