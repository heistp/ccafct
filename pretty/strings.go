package pretty

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

var spaces = make([]byte, 128)

var dashes = make([]byte, 128)

var equals = make([]byte, 128)

func init() {
	for i, _ := range dashes {
		spaces[i] = ' '
		dashes[i] = '-'
		equals[i] = '='
	}
}

func JoinInts(x []int, sep string) string {
	strs := make([]string, len(x))
	for i, v := range x {
		strs[i] = strconv.Itoa(v)
	}
	return strings.Join(strs, sep)
}

func JoinDurations(x []time.Duration, sep string) string {
	strs := make([]string, len(x))
	for i, v := range x {
		strs[i] = fmt.Sprintf("%s", v)
	}
	return strings.Join(strs, sep)
}

func Underline(w io.Writer, format string, args ...interface{}) {
	uline(dashes, w, format, args...)
}

func UnderlineDouble(w io.Writer, format string, args ...interface{}) {
	uline(equals, w, format, args...)
}

func uline(chars []byte, w io.Writer, format string, args ...interface{}) {
	s := strings.TrimSpace(fmt.Sprintf(format, args...))
	fmt.Fprintf(w, "%s\n", s)
	w.Write(chars[:len(s)])
	fmt.Fprintln(w)
}

func Float64(f float64, prec int) (s string) {
	s = strconv.FormatFloat(f, 'f', prec, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return
}
