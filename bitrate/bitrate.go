package bitrate

import (
	"fmt"

	"github.com/heistp/fct/pretty"
)

// Bitrate is a bitrate in bits per second.
type Bitrate int64

const (
	BPS  Bitrate = 1
	Kbps         = 1000 * BPS
	Mbps         = 1000 * Kbps
	Gbps         = 1000 * Mbps
	Tbps         = 1000 * Gbps
)

var qdiscUnits = map[string]string{
	"K": "Kbit",
	"M": "Mbit",
	"G": "Gbit",
	"T": "Tbit",
}

var stdUnits = map[string]string{
	"K": "Kbps",
	"M": "Mbps",
	"G": "Gbps",
	"T": "Tbps",
}

// Kbps returns the Bitrate in kilobits per second.
func (b Bitrate) Kbps() float64 {
	return float64(b) / float64(Kbps)
}

// Mbps returns the Bitrate in megabits per second.
func (b Bitrate) Mbps() float64 {
	return float64(b) / float64(Mbps)
}

// Gbps returns the Bitrate in gigabits per second.
func (b Bitrate) Gbps() float64 {
	return float64(b) / float64(Gbps)
}

// Tbps returns the Bitrate in terabits per second.
func (b Bitrate) Tbps() float64 {
	return float64(b) / float64(Tbps)
}

// Qdisc returns a formatted string suitable for Linux qdisc parameters.
func (b Bitrate) Qdisc() string {
	return b.format(qdiscUnits)
}

func (b Bitrate) String() string {
	return b.format(stdUnits)
}

func (b Bitrate) format(units map[string]string) string {
	switch {
	case b < 1*Kbps:
		return fmt.Sprintf("%dbps", b)
	case b < 10*Kbps:
		return pretty.Float64(b.Kbps(), 3) + units["K"]
	case b < 100*Kbps:
		return pretty.Float64(b.Kbps(), 2) + units["K"]
	case b < 1*Mbps:
		return pretty.Float64(b.Kbps(), 1) + units["K"]
	case b < 10*Mbps:
		return pretty.Float64(b.Mbps(), 3) + units["M"]
	case b < 100*Mbps:
		return pretty.Float64(b.Mbps(), 2) + units["M"]
	case b < 1*Gbps:
		return pretty.Float64(b.Mbps(), 1) + units["M"]
	case b < 10*Gbps:
		return pretty.Float64(b.Gbps(), 3) + units["G"]
	case b < 100*Gbps:
		return pretty.Float64(b.Gbps(), 2) + units["G"]
	case b < 1*Tbps:
		return pretty.Float64(b.Gbps(), 1) + units["G"]
	default:
		return pretty.Float64(b.Tbps(), 3) + units["T"]
	}
}
