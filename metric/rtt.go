package metric

import (
	"fmt"

	"github.com/heistp/fct/harm"
)

type RTT struct {
	Duration
	Harm harm.Harm
}

func (r *RTT) SetHarm(solo RTT) {
	r.Harm = harm.LessIsBetter(float64(solo.Duration), float64(r.Duration))
}

func (r RTT) String() string {
	if r.Harm.Zero() {
		return r.Duration.String()
	}

	return fmt.Sprintf("%s (%s)", r.Duration, r.Harm)
}
