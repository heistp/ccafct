package metric

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/heistp/fct/pretty"
)

type Duration time.Duration

func Ms(ms int) Duration {
	return Duration(time.Duration(ms) * time.Millisecond)
}

func (d Duration) FormatMillis(prec int, units bool) string {
	u := ""
	if units {
		u = "ms"
	}
	ms := time.Duration(d).Seconds() * 1000
	return fmt.Sprintf("%s%s", strconv.FormatFloat(ms, 'f', prec, 64), u)
}

func (d Duration) String() string {
	ms := time.Duration(d).Seconds() * 1000
	return fmt.Sprintf("%sms", pretty.Float64(ms, 1))
}

func JoinDuration(x []Duration, sep string) string {
	strs := make([]string, len(x))
	for i, v := range x {
		strs[i] = v.String()
	}
	return strings.Join(strs, sep)
}
