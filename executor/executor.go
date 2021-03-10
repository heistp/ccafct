// Package executor makes execution of multiple commands easier.
package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

const slurpBufSize = 4096

// Trace enables tracing globally.
var Trace bool

// Spec is a Job specification.
type Spec struct {
	Context      context.Context
	Stdin        []byte
	Background   bool
	NoWait       bool
	IgnoreErrors bool
	Log          bool
	LogStdout    bool
	LogStderr    bool
	Emit         bool
	EmitStdout   bool
	EmitStderr   bool
}

// Job is a command, its Spec and the results.
type Job struct {
	Command *exec.Cmd
	Spec
	Stdout bytes.Buffer
	Stderr bytes.Buffer
	Err    error
	sync.WaitGroup
}

func (j *Job) slurp(stdin io.WriteCloser, stdout, stderr io.ReadCloser) {
	defer j.Done()

	if stdin != nil {
		j.Add(1)
		go func() {
			defer j.Done()
			defer stdin.Close()
			if _, err := stdin.Write(j.Stdin); err != nil {
				log.Printf("error writing stdin: '%s'", err)
			}
		}()
	}

	doSlurp := func(r io.Reader, buf *bytes.Buffer, emit, lg bool,
		name string) (err error) {
		var n int
		b := make([]byte, slurpBufSize)
		for {
			n, err = r.Read(b)
			buf.Write(b[:n])
			if emit {
				os.Stderr.Write(b[:n])
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("error reading %s: '%s'", name, err.Error())
				}
				break
			}
		}
		if lg && buf.Len() > 0 {
			log.Printf("%s for '%s'\n%s", name, j.Command, buf.String())
		}
		return
	}

	j.Add(1)
	go func() {
		defer j.Done()
		emit := j.Emit || j.EmitStderr
		lg := j.Log || j.LogStderr
		doSlurp(stderr, &j.Stderr, emit, lg, "stderr")
	}()

	j.Add(1)
	go func() {
		defer j.Done()
		emit := j.Emit || j.EmitStdout
		lg := j.Log || j.LogStdout
		doSlurp(stdout, &j.Stdout, emit, lg, "stdout")
	}()
}

// Executor creates and runs Jobs for each command. It is not safe for
// concurrent use.
type Executor struct {
	Trace        bool
	IgnoreErrors bool
	NoLogErrors  bool
	Job          []*Job
	Errors       int
	waited       bool
}

func (e *Executor) Run(cmd string, arg ...string) *Job {
	return e.RunSpec(Spec{}, cmd, arg...)
}

func (e *Executor) Runf(format string, arg ...interface{}) *Job {
	return e.RunSpecf(Spec{}, format, arg...)
}

func (e *Executor) Emit(cmd string, arg ...string) *Job {
	return e.RunSpec(Spec{Emit: true}, cmd, arg...)
}

func (e *Executor) Emitf(format string, arg ...interface{}) *Job {
	return e.RunSpecf(Spec{Emit: true}, format, arg...)
}

func (e *Executor) RunSpec(spec Spec, cmd string, arg ...string) *Job {
	var c *exec.Cmd
	if spec.Context != nil {
		c = exec.CommandContext(spec.Context, cmd, arg...)
	} else {
		c = exec.Command(cmd, arg...)
	}
	return e.RunCommand(c, spec)
}

func (e *Executor) RunSpecf(spec Spec, format string, arg ...interface{}) *Job {
	s := fmt.Sprintf(format, arg...)
	f := strings.Split(s, " ")
	return e.RunSpec(spec, f[0], f[1:]...)
}

// Err returns an error if any of the jobs had nonzero exit status.
func (e *Executor) Err() (err error) {
	if e.Errors > 0 {
		err = fmt.Errorf("%d jobs with nonzero exit status", e.Errors)
	}
	return
}

// Wait waits for all jobs to complete.
func (e *Executor) Wait() {
	if e.waited {
		return
	}
	e.waited = true

	for _, j := range e.Job {
		if !j.Background || j.NoWait {
			continue
		}
		j.Wait()
		if err := j.Command.Wait(); err != nil {
			e.onError(j, err)
		}
	}
}

// Interrupt sends an interrupt to any background processes.
func (e *Executor) Interrupt() {
	for _, j := range e.Job {
		if j.Background {
			j.Command.Process.Signal(syscall.SIGINT)
		}
	}
}

// Kill kills any background processes.
func (e *Executor) Kill() {
	for _, j := range e.Job {
		if j.Background {
			j.Command.Process.Kill()
		}
	}
}

// RunCommand runs the specified command and job Spec.
func (e *Executor) RunCommand(cmd *exec.Cmd, spec Spec) (job *Job) {
	if e.Job == nil {
		e.Job = make([]*Job, 0)
	}

	if e.Errors > 0 && !e.IgnoreErrors {
		return
	}

	// inherit environment and exit on parent exit (Linux only)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM,
	}

	job = &Job{Command: cmd, Spec: spec}

	e.Job = append(e.Job, job)

	if e.Trace || Trace {
		log.Printf("%s", job.Command)
	}

	var err error

	var stdin io.WriteCloser
	if job.Stdin != nil {
		if stdin, err = cmd.StdinPipe(); err != nil {
			e.onError(job, err)
			return
		}
	}

	var stdout io.ReadCloser
	if stdout, err = cmd.StdoutPipe(); err != nil {
		e.onError(job, err)
		return
	}

	var stderr io.ReadCloser
	if stderr, err = cmd.StderrPipe(); err != nil {
		e.onError(job, err)
		return
	}

	if err = cmd.Start(); err != nil {
		e.onError(job, err)
		return
	}

	if job.Background {
		job.Add(1)
		job.slurp(stdin, stdout, stderr)
		return
	}

	job.Add(1)
	job.slurp(stdin, stdout, stderr)
	job.Wait()

	if err = job.Command.Wait(); err != nil {
		e.onError(job, err)
		return
	}

	return
}

// onError is called internally when an error occurs.
func (e *Executor) onError(job *Job, err error) {
	if !job.IgnoreErrors {
		job.Err = err
		e.Errors++
	}
	if !e.NoLogErrors {
		log.Printf("'%s' failed, %s", job.Command, err)
		if job.Stderr.Len() > 0 {
			log.Printf("stderr was: %s", job.Stderr.String())
		}
	}
}
