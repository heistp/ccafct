package main

import "github.com/heistp/fct/executor"

func main() {
	ex := new(executor.Executor)

	spec := executor.Spec{Background: true}
	ex.RunSpecf(spec, "sleep 60")
}
