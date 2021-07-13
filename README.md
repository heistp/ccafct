ccafct - Congestion Control Algorithm Flow Completion Time Test
===============================================================

This tests FCT (flow completion time) for a baseline CCA (congestion
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
solo is the baseline, without competition.

Installation
------------

1. Install [Go](https://golang.org/dl/) and iperf3.
2. Download and build the source:
```
go get github.com/heistp/ccafct
cd fct # source location
make
```

Running
-------

To run a test, run ccafct as root, specifying the CCAs to test, e.g.:

```
sudo ./ccafct -cca cubic,prague
```

To change parameters other than the CCAs under test, it's currently
necessary to modify the globals in `cmd/ccafct/main.go`.

Sample Output
-------------

Sample output for TCP CUBIC and TCP Prague is below.

```
Test Parameters:
----------------
CCAs under test:  cubic, prague
RTTs:             10ms, 20ms, 40ms, 80ms, 160ms
Bandwidth:        50Mbps
Qdisc:            fq_codel flows 1
Slow start delay: 20s

FCT Workload Parameters:
------------------------
Server URL:        localhost:8188
CCA:               cubic
Duration:          3m0s
Flows:             900
Mean arrival time: 200ms
Est. bandwidth:    25.83Mbps
Flow lengths:      
|- P5:             65536
|- Mean:           645683
|- P95:            2097152

RTT    CCA     GeoMean (Harm)    Median (Harm)     P95 (Harm)
---    ---     --------------    -------------     ----------
10ms   -       176.2ms           154.3ms           1061.3ms
10ms   cubic   339.3ms (0.481)   314.9ms (0.510)   2002.7ms (0.470)
10ms   prague  1171.3ms (0.850)  1131.7ms (0.864)  7305.2ms (0.855)
20ms   -       243.0ms           211.5ms           1155.3ms
20ms   cubic   376.6ms (0.355)   351.7ms (0.399)   1986.0ms (0.418)
20ms   prague  1394.1ms (0.826)  1385.3ms (0.847)  8454.8ms (0.863)
40ms   -       369.2ms           317.8ms           1634.7ms
40ms   cubic   475.7ms (0.224)   423.9ms (0.250)   2179.9ms (0.250)
40ms   prague  1643.8ms (0.775)  1637.7ms (0.806)  9879.3ms (0.835)
80ms   -       599.9ms           554.5ms           2135.6ms
80ms   cubic   906.7ms (0.338)   846.9ms (0.345)   3709.4ms (0.424)
80ms   prague  2384.8ms (0.748)  2228.4ms (0.751)  12159.9ms (0.824)
160ms  -       976.6ms           976.4ms           3465.1ms
160ms  cubic   1127.7ms (0.134)  1043.2ms (0.064)  4207.8ms (0.177)
160ms  prague  2471.9ms (0.605)  2174.7ms (0.551)  14418.8ms (0.760)
```
