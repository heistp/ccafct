package ccafct

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/heistp/fct/bitrate"
	"github.com/heistp/fct/pretty"
	"github.com/heistp/fct/unit"
	"gonum.org/v1/gonum/stat/distuv"
)

// countWriter counts and discards bytes.
type countWriter struct {
	Bytes unit.Bytes
}

func (w *countWriter) Write(p []byte) (n int, err error) {
	w.Bytes += unit.Bytes(len(p))
	return len(p), nil
}

var DefaultAddr = "localhost"

var DefaultCCA = "cubic"

var DefaultDuration = 10 * time.Second

var DefaultMeanArrival = 200 * time.Millisecond

var DefaultArrivalExpRate = 1.0

var DefaultLenP5 = 64 * unit.Kilobyte

var DefaultLenP95 = 2 * unit.Megabyte

// Params contains test parameters.
type Params struct {
	// Addr is the server addr:port.
	Addr string

	// CCA is the congestion control algorithm.
	CCA string

	// Duration is the test duration.
	Duration time.Duration

	// MeanArrival is the mean arrival time between requests.
	MeanArrival time.Duration

	// ArrivalExpRate is the rate parameter for the exponential arrival time
	// distribution.
	ArrivalExpRate float64

	// LenP5 is the 5th percentile of the lognormal flow length distribution.
	LenP5 unit.Bytes

	// LenP95 is the 95th percentile of the lognormal flow length distribution.
	LenP95 unit.Bytes

	// DisableGC disables the garbage collector during the test if set.
	DisableGC bool
}

func (p *Params) init() {
	if p.Addr == "" {
		p.Addr = DefaultAddr
	}
	if p.CCA == "" {
		p.CCA = DefaultCCA
	}
	if p.Duration == 0 {
		p.Duration = DefaultDuration
	}
	if p.MeanArrival == 0 {
		p.MeanArrival = DefaultMeanArrival
	}
	if p.ArrivalExpRate == 0 {
		p.ArrivalExpRate = DefaultArrivalExpRate
	}
	if p.LenP5 == 0 {
		p.LenP5 = DefaultLenP5
	}
	if p.LenP95 == 0 {
		p.LenP95 = DefaultLenP95
	}
}

// Test contains the test parameters and related test configuration.
type Test struct {
	Params

	// URL is the server URL
	URL string

	// Flows is the number of flows that will run.
	Flows int

	// ArrivalDist is the flow arrival distribution.
	ArrivalDist distuv.Exponential

	// LenDist is the flow length distribution.
	LenDist distuv.LogNormal

	// MeanFlowLen is the mean flow length.
	MeanFlowLen int

	// Bandwidth is the estimated bandwidth.
	Bandwidth bitrate.Bitrate

	sync.WaitGroup
}

// NewTest returns a new test given test parameters.
func NewTest(p Params) (t Test) {
	t.Params = p
	t.Params.init()

	// server URL
	f := strings.Split(t.Addr, ":")
	if len(f) == 1 {
		t.Addr = fmt.Sprintf("%s:%d", t.Addr, DefaultPort)
	}
	t.URL = fmt.Sprintf("http://%s%s", t.Addr, FCTPath)

	// number of flows
	t.Flows = int(t.Duration / t.MeanArrival)

	// arrival distribution
	t.ArrivalDist = distuv.Exponential{Rate: t.ArrivalExpRate}

	// flow length distribution
	log5 := math.Log(float64(t.LenP5))
	log95 := math.Log(float64(t.LenP95))
	mu := (log5 + log95) / 2
	sigma := (log95 - log5) / (2 * 1.645)
	t.LenDist = distuv.LogNormal{Mu: mu, Sigma: sigma}

	// calculate mean flow length and bandwidth
	rps := float64(1 * time.Second / t.MeanArrival)
	mfl := math.Exp(mu + 0.5*math.Pow(sigma, 2))
	t.MeanFlowLen = int(mfl)
	t.Bandwidth = bitrate.Bitrate(rps * mfl * 8)

	return
}

// emitTest emits the test parameters.
func (t Test) Emit(w io.Writer) {
	// log some things
	tw := pretty.NewTableWriter(w)
	tw.Printf("Server URL:\t%s", t.Addr)
	tw.Printf("CCA:\t%s", t.CCA)
	tw.Printf("Duration:\t%s", t.Duration)
	tw.Printf("Flows:\t%d", t.Flows)
	tw.Printf("Mean arrival time:\t%s", t.MeanArrival)
	tw.Printf("Est. bandwidth:\t%s", t.Bandwidth)
	tw.Printf("Flow lengths:\t")
	tw.Printf("|- P5:\t%d", t.LenP5)
	tw.Printf("|- Mean:\t%d", t.MeanFlowLen)
	tw.Printf("|- P95:\t%d", t.LenP95)
	tw.Flush()
}

// Run runs a test and returns the result.
func (t *Test) Run(ctx context.Context) (data Data, err error) {
	if t.DisableGC {
		runtime.GC()
		debug.SetGCPercent(-1)
	}

	ctx, cancel := context.WithCancel(ctx)

	data = newData()
	data.Start = time.Now()
	// the below could be more memory efficient for large flow counts, but we
	// don't want to risk that goroutines can't exit on error
	errCh := make(chan error, t.Flows)

loop:
	for i := 0; i < t.Flows; i++ {
		if i > 0 {
			waitNs := t.ArrivalDist.Rand() * float64(t.MeanArrival)
			wait := time.Duration(waitNs) * time.Nanosecond
			select {
			case <-ctx.Done():
				log.Printf("client context: '%s'", ctx.Err())
				break loop
			case <-time.After(wait):
			case err = <-errCh:
				cancel()
				break loop
			}
		}

		reqLen := int(t.LenDist.Rand())
		t.Add(1)
		go func(reqLen int, errCh chan error) {
			var flow Flow
			var rerr error
			if flow, rerr = t.doRequest(ctx, reqLen); rerr != nil {
				errCh <- rerr
				return
			}
			data.AddFlow(flow)
		}(reqLen, errCh)
	}

	t.Wait()

	data.End = time.Now()

	if t.DisableGC {
		debug.SetGCPercent(100)
		runtime.GC()
	}

	return
}

func (t *Test) doRequest(ctx context.Context, reqLen int) (flow Flow, err error) {
	defer func() {
		t.Done()
	}()

	client := &http.Client{}
	defer client.CloseIdleConnections()

	var req *http.Request
	if req, err = http.NewRequest("GET", t.URL, nil); err != nil {
		return
	}
	req.Header.Add(FlowLengthHeader, strconv.Itoa(reqLen))
	req = req.WithContext(ctx)

	if t.CCA != "" {
		req.Header.Add(CCAHeader, t.CCA)
	}

	flow.Start = time.Now()

	var resp *http.Response
	if resp, err = client.Do(req); err != nil {
		return
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("client received: %s (%d)",
			resp.Status, resp.StatusCode)
		return
	}

	cw := new(countWriter)
	if err = resp.Write(cw); err != nil {
		return
	}

	flow.End = time.Now()
	flow.Length = cw.Bytes

	return
}
