package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

func main() {
	iterations := flag.Int("iterations", 10, "measured command iterations")
	warmups := flag.Int("warmups", 3, "warmup command iterations")
	flag.Parse()
	command := flag.Args()
	if len(command) == 0 || *iterations < 1 || *warmups < 0 {
		flag.Usage()
		os.Exit(2)
	}

	for range *warmups {
		if err := run(command); err != nil {
			fatal(err)
		}
	}

	started := time.Now()
	for range *iterations {
		if err := run(command); err != nil {
			fatal(err)
		}
	}
	elapsed := time.Since(started)
	averageMS := float64(elapsed) / float64(time.Millisecond) / float64(*iterations)
	fmt.Printf("%.3f\n", averageMS)
}

func run(command []string) error {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
