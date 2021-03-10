package ccafct

import (
	"sync"
	"time"

	"github.com/heistp/fct/unit"
)

const flowInitCap = 16384

type EventType int

const (
	EventTypeStart EventType = iota
	EventTypeStop
)

// Flow contains data for one flow.
type Flow struct {
	// Start is the flow start time.
	Start time.Time

	// End is the flow end time.
	End time.Time

	// Length is the flow length.
	Length unit.Bytes
}

// Duration returns the flow duration.
func (f Flow) Duration() time.Duration {
	return f.End.Sub(f.Start)
}

// Data contains data gathered during a test.
type Data struct {
	// Flow contains the flow data.
	Flow []Flow

	// Start is the test start time.
	Start time.Time

	// End is the test end time.
	End time.Time

	sync.Mutex
}

func newData() Data {
	return Data{
		make([]Flow, 0, flowInitCap),
		time.Time{},
		time.Time{},
		sync.Mutex{},
	}
}

// AddFlow adds flow data.
func (d *Data) AddFlow(f Flow) {
	d.Lock()
	defer d.Unlock()
	d.Flow = append(d.Flow, f)
}

// FlowDurations returns a slice of all flow durations.
func (d *Data) FlowDurations() (durs []time.Duration) {
	durs = make([]time.Duration, len(d.Flow))
	for i, f := range d.Flow {
		durs[i] = f.Duration()
	}
	return
}
