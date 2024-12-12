package main

import (
	"fmt"
	"os"
)

var help_message = `
usage: proto-merge <path to proto file A> <path to proto file B>

Merge A into B.
`

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "it takes exact 2 arguments, filepath to proto files to merge")
		fmt.Fprintln(os.Stderr, help_message)
		os.Exit(1)
		return
	}

	a, err := NewInventoryFromFile(os.Args[1])
	if err != nil {
		errExit(err)
	}
	b, err := NewInventoryFromFile(os.Args[2])
	if err != nil {
		errExit(err)
	}

	if err := a.MergeOut(b, os.Stdout); err != nil {
		errExit(err)
	}
}

func errExit(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
