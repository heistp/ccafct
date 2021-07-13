package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	ccafct "github.com/heistp/fct"
	"github.com/heistp/fct/bitrate"
	"github.com/heistp/fct/executor"
	"github.com/heistp/fct/metric"
	"github.com/heistp/fct/netns"
	"github.com/heistp/fct/pretty"
	"github.com/heistp/fct/unit"
)

// RTT is the RTTs to test.
var RTT = []metric.Duration{
	metric.Ms(10),
	metric.Ms(20),
	metric.Ms(40),
	metric.Ms(80),
	metric.Ms(160),
}

// Bandwidth is the simulated bottleneck link bandwidth.
var Bandwidth = 50 * bitrate.Mbps

// Qdisc is the queueing discipline to use at the bottleneck.
var Qdisc = "fq_codel flows 1"

// CCA are the congestion control algorithms to test.
var CCA = []string{}

// FCTDur is the duration to run the FCT test.
var FCTDur = 3 * time.Minute

// FCTMeanArrival is the mean time between new flow arrivals.
var FCTMeanArrival = 200 * time.Millisecond

// FCTLenP5 is the 5th percentile flow length in the lognormal distribution.
var FCTLenP5 = 64 * unit.Kilobyte

// FCTLenP95 is the 95th percentile flow length in the lognormal distribution.
var FCTLenP95 = 2 * unit.Megabyte

// FCTTimeout is how long to wait after FCTDur for the FCT test to complete.
var FCTTimeout = 1 * time.Minute

// ContextTimeout is how long to wait after executing the test tools to timeout.
var ContextTimeout = 30 * time.Second

// FCTCCA is the CC algorithm to use for all FCT flows.
var FCTCCA = "cubic"

// SlowStartDelay is a delay long enough for the CCA to exit slow start.
var SlowStartDelay = 20 * time.Second

// DefaultCompetitionCCA is the default long-running CCAs to test.
const DefaultCompetitionCCA = "cubic"

// Description is emitted in the usage and test output.
const Description = `This tests FCT (flow completion time) for a baseline CCA (congestion
control algorithm) through a single Codel queue, with and without the
competition of a single flow from a selected competing CCA, and
measures the resulting harm to FCT. Network namespaces are used to
simulate path delay.

The concept of harm is further defined here:
https://www.cs.cmu.edu/~rware/assets/pdf/ware-hotnets19.pdf

The test process is as follows:

1. Run a test FCT workload with the baseline CCA. This gives us the
   solo performance, without competition. Harm calculations are not
   made for this step, as there is no competition yet.
2. Run the same FCT workload in competition with a single flow from
   one of the competitor CCAs, as follows:
   * Start one flow for the competing CCA, giving it enough time to
     exit the slow-start phase.
   * Start the FCT workload for the baseline CCA.
   * Wait for the baseline CCA to complete, or a timeout to expire.
   * Terminate the competing CCA flow.
   * Calculate the resulting FCT statistics and harm to FCT.
3. Run step 2 for each additional competitor CCA, sequentially.

Multiple CCAs are tested sequentially, across multiple RTTs. The CCAs
under test may be specified using the -cca flag at the command line.
The FCT workload introduces flows with an exponential distribution,
and chooses flow lengths with a lognormal distribution. These and
other parameter changes must be made by modifying the globals at the
top of the program's source code.

The harm calculations quantify the CCA's impact on the FCT results. As
a "less is better" metric, FCT harm is calculated as:

(workload - solo) / workload

where workload is the FCT in competition with the CCA under test, and
solo is the baseline, without competition.`

// SetTestMode changes defaults to be suitable for a quick test.
func SetTestMode() {
	RTT = []metric.Duration{metric.Ms(10), metric.Ms(20)}
	FCTDur = 5 * time.Second
	SlowStartDelay = 0
}

// DelayQdisc returns the qdisc used to simulate delay.
func DelayQdisc(rtt metric.Duration) string {
	d := rtt / 2
	return fmt.Sprintf("netem delay %s limit 1000000", d)
}

// SoloID identifies the demand traffic, without a competing CCA.
const SoloID = "-"

// Result is one test result.
type Result struct {
	RTT metric.Duration
	CCA string
	ccafct.Stats
}

// setupRig sets up the netns test rig.
func setupRig(rtt metric.Duration) (rig *netns.Rig, err error) {
	// set up 2+2+2 rig
	rig = &netns.Rig{
		LeftEndpoints:  2,
		Middleboxes:    2,
		RightEndpoints: 2,
	}
	defer func() {
		if err != nil {
			rig.Teardown()
		}
	}()
	if err = rig.Setup(); err != nil {
		return
	}

	delayQdisc := DelayQdisc(rtt)

	// set up middleboxes(two middleboxes using only egress qdiscs)
	m0 := rig.MidNs(0)
	m1 := rig.MidNs(1)
	if err = rig.AddRootQdisc(m0, rig.RightDev(m0), delayQdisc); err != nil {
		return
	}
	if err = rig.AddHTBQdisc(m1, rig.RightDev(m1), Qdisc, Bandwidth); err != nil {
		return
	}
	if err = rig.AddRootQdisc(m1, rig.LeftDev(m1), delayQdisc); err != nil {
		return
	}
	if err = rig.AddHTBQdisc(m0, rig.LeftDev(m0), Qdisc, Bandwidth); err != nil {
		return
	}

	// set up endpoints
	l0 := rig.LeftNs(0)
	r0 := rig.RightNs(0)
	l1 := rig.LeftNs(1)
	r1 := rig.RightNs(1)

	// set up ECN
	ex := new(executor.Executor)
	ex.Runf("ip netns exec %s sysctl -w net.ipv4.tcp_ecn=3", l0)
	ex.Runf("ip netns exec %s sysctl -w net.ipv4.tcp_ecn=3", r0)
	ex.Runf("ip netns exec %s sysctl -w net.ipv4.tcp_ecn=2", l1)
	ex.Runf("ip netns exec %s sysctl -w net.ipv4.tcp_ecn=2", r1)

	// do ping to test and warm up arp
	spec := executor.Spec{Log: true}
	ex.RunSpecf(spec, "ip netns exec %s ping -c 2 -i 0.1 %s", l0, rig.RightIP(0))
	ex.RunSpecf(spec, "ip netns exec %s ping -c 2 -i 0.1 %s", l1, rig.RightIP(1))

	err = ex.Err()

	return
}

// runTest runs a test.
func runTest(rig *netns.Rig, testJSON []byte, cca string) (data ccafct.Data, err error) {
	ex := new(executor.Executor)

	if cca != SoloID {
		t := SlowStartDelay + FCTDur + FCTTimeout
		spec := executor.Spec{
			Background:   true,
			Log:          true,
			IgnoreErrors: true,
		}
		ex.RunSpecf(spec, "ip netns exec %s iperf3 -R -C %s -t %d -c %s",
			rig.LeftNs(0), cca, int(t.Seconds()), rig.RightIP(0))
		time.Sleep(SlowStartDelay)
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		FCTDur+FCTTimeout+ContextTimeout)
	defer cancel()

	testJob := ex.RunSpecf(executor.Spec{
		Stdin:   testJSON,
		Context: ctx,
	}, "ip netns exec %s ./fct json", rig.LeftNs(1))

	ex.Interrupt()
	ex.Wait()
	if err = ex.Err(); err != nil {
		return
	}

	// unmarshal data and get stats
	if json.Unmarshal(testJob.Stdout.Bytes(), &data); err != nil {
		return
	}

	return
}

// runRTT runs one RTT across the CC algos.
func runRTT(rtt metric.Duration) (result []Result, err error) {
	// set up rig
	var rig *netns.Rig
	if rig, err = setupRig(rtt); err != nil {
		return
	}
	defer func() {
		rig.Teardown()
	}()

	// create test
	test := ccafct.NewTest(ccafct.Params{
		Addr:        rig.RightIP(1),
		CCA:         FCTCCA,
		Duration:    FCTDur,
		MeanArrival: FCTMeanArrival,
		LenP5:       FCTLenP5,
		LenP95:      FCTLenP95,
	})

	// start servers
	ex := new(executor.Executor)
	defer ex.Kill()
	r0 := rig.RightNs(0)
	r1 := rig.RightNs(1)
	ex.RunSpecf(executor.Spec{Background: true, NoWait: true},
		"ip netns exec %s iperf3 -s", r0)
	ex.RunSpecf(executor.Spec{Background: true, NoWait: true},
		"ip netns exec %s ./fct server", r1)
	time.Sleep(200 * time.Millisecond)

	// create test JSON
	var testJSON []byte
	if testJSON, err = json.Marshal(test); err != nil {
		return
	}

	// solo test
	log.Printf("running %s solo", rtt)
	var data ccafct.Data
	if data, err = runTest(rig, testJSON, SoloID); err != nil {
		return
	}
	var solo ccafct.Stats
	if solo, err = ccafct.Analyze(data); err != nil {
		return
	}
	result = append(result, Result{rtt, SoloID, solo})

	// CCA tests
	for _, cca := range CCA {
		log.Printf("running %s %s", rtt, cca)
		if data, err = runTest(rig, testJSON, cca); err != nil {
			return
		}
		var stats ccafct.Stats
		if stats, err = ccafct.Analyze(data); err != nil {
			return
		}
		stats.SetHarm(solo)
		result = append(result, Result{rtt, cca, stats})
	}

	return
}

// run runs the test.
func run() (err error) {
	pretty.UnderlineDouble(os.Stdout,
		"Congestion Control Algorithm Flow Completion Time Test")
	fmt.Println()
	fmt.Printf("%s\n", Description)
	fmt.Println()

	// emit test config
	pretty.Underline(os.Stdout, "Test Parameters:")
	tw := pretty.NewTableWriter(os.Stdout)
	tw.Row("CCAs under test:", strings.Join(CCA, ", "))
	tw.Printf("RTTs:\t%s", metric.JoinDuration(RTT, ", "))
	tw.Row("Bandwidth:", Bandwidth)
	tw.Row("Qdisc:", Qdisc)
	tw.Row("Slow start delay:", SlowStartDelay)
	tw.Flush()

	// create sample FCT test and emit config
	fmt.Println()
	pretty.Underline(os.Stdout, "FCT Workload Parameters:")
	ccafct.NewTest(ccafct.Params{
		Duration: FCTDur,
	}).Emit(os.Stdout)

	// run each RTT and add results
	var result []Result
	for _, rtt := range RTT {
		var res []Result
		if res, err = runRTT(rtt); err != nil {
			return
		}
		result = append(result, res...)
	}

	// emit results
	fmt.Println()
	tw = pretty.NewTableWriterPad(os.Stdout, 2, "")
	tw.URow("RTT", "CCA", "GeoMean (Harm)", "Median (Harm)", "P95 (Harm)")
	for _, r := range result {
		tw.Row(r.RTT, r.CCA, r.GeoMean, r.Median, r.P95)
	}
	tw.Flush()

	return
}

// main entry point.
func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stderr)
	executor.Trace = true

	// process flags
	var cca string
	var testMode bool
	flag.Usage = func() {
		w := flag.CommandLine.Output()
		fmt.Fprintf(w, "usage: %s [options]\n", os.Args[0])
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Description:")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s\n", Description)
	}
	flag.StringVar(&cca, "cca", DefaultCompetitionCCA,
		"comma separated list of CCAs to test for the competition flow")
	flag.BoolVar(&testMode, "t", false, "perform quick test to verify setup")
	flag.Parse()
	for _, c := range strings.Split(cca, ",") {
		CCA = append(CCA, strings.TrimSpace(c))
	}
	if testMode {
		SetTestMode()
	}

	// run the test
	if err := run(); err != nil {
		log.Fatalf("ERROR: %s", err)
	}
}
