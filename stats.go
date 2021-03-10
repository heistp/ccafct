package ccafct

import (
	"fmt"
	"io"
	"sort"

	"github.com/heistp/fct/metric"
	"github.com/heistp/fct/pretty"
	"gonum.org/v1/gonum/stat"
)

// Stats contains the test statistics.
type Stats struct {
	// GeoMean is the geometric mean value.
	GeoMean metric.FCT

	// Median is the median value.
	Median metric.FCT

	// P95 is the 95th percentile value.
	P95 metric.FCT
}

// Analyze analyzes the data to produce stats.
func Analyze(d Data) (stats Stats, err error) {
	// durations to floats
	durs := d.FlowDurations()
	if len(durs) == 0 {
		err = fmt.Errorf("unable to analyze empty flow durations")
		return
	}

	f := make([]float64, len(durs))
	for i, d := range durs {
		f[i] = float64(d)
	}
	sort.Float64s(f)

	geomean := stat.GeometricMean(f, nil)
	median := stat.Quantile(0.5, stat.Empirical, f, nil)
	p95 := stat.Quantile(0.95, stat.Empirical, f, nil)

	stats = Stats{
		GeoMean: metric.FCTFromFloat64(geomean),
		Median:  metric.FCTFromFloat64(median),
		P95:     metric.FCTFromFloat64(p95),
	}
	return
}

// SetHarm sets harm stats relative to solo performance.
func (s *Stats) SetHarm(solo Stats) {
	s.GeoMean.SetHarm(solo.GeoMean)
	s.Median.SetHarm(solo.Median)
	s.P95.SetHarm(solo.P95)
}

// Emit print the stats in text form.
func (s *Stats) Emit(w io.Writer) {
	tw := pretty.NewTableWriter(w)
	tw.Printf("")
	tw.Printf("GeoMean:\t%s", s.GeoMean)
	tw.Printf("Median:\t%s", s.Median)
	tw.Printf("P95:\t%s", s.P95)
	tw.Flush()
}
