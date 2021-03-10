package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	ccafct "github.com/heistp/fct"
)

type Mode int

const (
	ClientMode Mode = iota
	ServerMode
)

// fail logs an error and exits
func fail(f string, a ...interface{}) {
	log.Fatalf("ERROR: "+f, a...)
}

// usage emits program usage
func usage(w io.Writer) {
	fmt.Fprintf(w, "usage: fct client addr[:port] | server | json\n")
}

// runClient runs the client.
func runClient(addr string) (err error) {
	p := ccafct.Params{}
	p.Addr = addr
	t := ccafct.NewTest(p)

	t.Emit(os.Stdout)

	var data ccafct.Data
	if data, err = t.Run(context.Background()); err != nil {
		return
	}
	var stats ccafct.Stats
	if stats, err = ccafct.Analyze(data); err != nil {
		return
	}
	stats.Emit(os.Stdout)

	return
}

// runServer runs the server.
func runServer() error {
	s := new(ccafct.Server)
	return s.Run()
}

// runJSON runs JSON mode.
func runJSON() (err error) {
	// test Test from stdin
	var test ccafct.Test
	br := bufio.NewReader(os.Stdin)
	dec := json.NewDecoder(br)
	if err = dec.Decode(&test); err != nil {
		return
	}

	// run Test
	var data ccafct.Data
	if data, err = test.Run(context.Background()); err != nil {
		return
	}

	// send Data to stdout
	bw := bufio.NewWriter(os.Stdout)
	defer bw.Flush()
	enc := json.NewEncoder(bw)
	if err = enc.Encode(&data); err != nil {
		return
	}

	return
}

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(-1)
	}

	cmd := os.Args[1]

	var err error

	switch cmd {
	case "client":
		if len(os.Args) < 3 {
			fail("client requires addr:port argument")
		}
		err = runClient(os.Args[2])
	case "server":
		err = runServer()
	case "json":
		err = runJSON()
	default:
		fail("unknown command '%s'", cmd)
	}

	if err != nil {
		fail(err.Error())
	}
}
