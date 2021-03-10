package harm

import (
	"fmt"
	"math"
)

// Infinity is the harm returned when the workload is zero, and is invalid.
var Infinity = Harm(math.Inf(+1))

// Harm is a harm value according to the paper at:
// https://www.cs.cmu.edu/~rware/assets/pdf/ware-hotnets19.pdf
type Harm float64

func (h Harm) Invalid() bool {
	return h < 0 || h > 1
}

func (h Harm) Zero() bool {
	return h == 0
}

func (h Harm) Nonzero() bool {
	return h > 0 && !h.Invalid()
}

func (h Harm) String() string {
	if h.Invalid() {
		return fmt.Sprintf("!(%.3f)", h)
	}
	return fmt.Sprintf("%.3f", h)
}

// LessIsBetter returns the Harm for workload vs solo for a "less is better"
// metric.
func LessIsBetter(solo, workload float64) Harm {
	if workload == 0 {
		return Infinity
	}
	if workload < solo {
		return 0
	}
	return Harm((workload - solo) / workload)
}

// MoreIsBetter returns the Harm for workload vs solo for a "more is better"
// metric.
func MoreIsBetter(solo, workload float64) Harm {
	if solo == 0 {
		return Infinity
	}
	if workload > solo {
		return 0
	}
	return Harm((solo - workload) / solo)
}
