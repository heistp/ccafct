package metric

import (
	"fmt"

	"github.com/heistp/fct/harm"
)

type FCT struct {
	Duration
	Harm harm.Harm
}

func FCTFromFloat64(f float64) FCT {
	return FCT{
		Duration(f),
		0,
	}
}

func (f *FCT) SetHarm(solo FCT) {
	f.Harm = harm.LessIsBetter(float64(solo.Duration), float64(f.Duration))
}

func (f FCT) String() string {
	mstr := f.FormatMillis(1, true)
	if f.Harm.Zero() {
		return mstr
	}
	return fmt.Sprintf("%s (%s)", mstr, f.Harm)
}
