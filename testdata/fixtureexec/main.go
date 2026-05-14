package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "stream" {
		fmt.Println("fixture-stream")
		fmt.Fprintln(os.Stderr, "fixture-err")
		return
	}

	arg := ""
	if len(os.Args) > 1 {
		arg = os.Args[1]
	}

	fmt.Printf("%s|%s|%s\n", os.Getenv("EXEC_TEST_TOKEN"), mustWd(), arg)
}

func mustWd() string {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return wd
}
