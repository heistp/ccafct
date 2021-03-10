// Package netns sets up network configurations using Linux network namespaces.
package netns

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/heistp/fct/bitrate"
	"github.com/heistp/fct/executor"
)

const alphaNum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

const netPrefixLen = 24

const randomPrefixLen = 7

const (
	rightNamePrefix    = "r"
	midNamePrefix      = "m"
	leftNamePrefix     = "l"
	rightIPPrefix      = "10.12.1."
	leftIPPrefix       = "10.12.0."
	rightBackhaulIP    = "10.12.2.2"
	leftBackhaulIP     = "10.12.2.1"
	rightBackhaulIPNet = rightBackhaulIP + "/24"
	leftBackhaulIPNet  = leftBackhaulIP + "/24"
)

func init() {
	//executor.Trace = true
}

// Rig is an netns setup consisting of E endpoints at each end of a path, and M
// bridged middleboxes, where E > 0 and M > 0. The directions left and right
// are used for arbitrary orientation, to identify left endpoints, right
// endpoints, and the interfaces through the middleboxes in between.
//
// The default setup is a dumbbell, consisting of one endpoint at each end, and
// one middlebox.
//
// Once Setup is called, Teardown should be called in a defer block so the
// namespaces are deleted. Rigs must not be reused.
//
// A signal handler will also call Teardown on os.Interrupt or os.Kill, although
// the latter is not caught on Linux, so namespaces may still exist after
// unclean shutdowns.
type Rig struct {
	// LeftEndpoints is the number of left endpoints (defaults to 1).
	LeftEndpoints int

	// Middleboxes is the number of middleboxes (defaults to 1).
	Middleboxes int

	// RightEndpoints is the number of right endpoints (defaults to 1).
	RightEndpoints int

	leftNamePrefix string

	midNamePrefix string

	rightNamePrefix string

	namespaces []string

	done chan struct{}

	closed bool

	sync.Mutex
}

func (r *Rig) init() {
	r.done = make(chan struct{})
	r.closed = false
	if r.LeftEndpoints == 0 {
		r.LeftEndpoints = 1
	}
	if r.Middleboxes == 0 {
		r.Middleboxes = 1
	}
	if r.RightEndpoints == 0 {
		r.RightEndpoints = 1
	}

	randSuffix := randomPrefix()
	r.leftNamePrefix = fmt.Sprintf("%s.%s", leftNamePrefix, randSuffix)
	r.midNamePrefix = fmt.Sprintf("%s.%s", midNamePrefix, randSuffix)
	r.rightNamePrefix = fmt.Sprintf("%s.%s", rightNamePrefix, randSuffix)
}

// Setup sets up the namespaces in the Rig.
func (r *Rig) Setup() (err error) {
	r.init()
	defer func() {
		if err != nil {
			r.Teardown()
		}
	}()

	if err = r.setupRight(); err != nil {
		return
	}

	if err = r.setupMid(); err != nil {
		return
	}

	if err = r.setupLeft(); err != nil {
		return
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		for {
			select {
			case sig := <-c:
				log.Printf("%s received, tearing down rig", sig)
				r.Teardown()
			case <-r.done:
				return
			}
		}
	}()

	return
}

// AddHTBQdisc adds an HTB qdisc.
func (r *Rig) AddHTBQdisc(name, dev, qdisc string,
	bandwidth bitrate.Bitrate) error {
	ex := new(executor.Executor)
	ex.Runf("ip netns exec %s tc qdisc add dev %s root handle 1: htb default 1",
		name, dev)
	ex.Runf("ip netns exec %s tc class add dev %s parent 1: classid 1:1 htb rate %s ceil %s",
		name, dev, bandwidth.Qdisc(), bandwidth.Qdisc())
	ex.Runf("ip netns exec %s tc qdisc add dev %s parent 1:1 %s",
		name, dev, qdisc)
	return ex.Err()
}

// AddRootQdisc adds a root qdisc.
func (r *Rig) AddRootQdisc(name, dev, qdisc string) error {
	ex := new(executor.Executor)
	ex.Runf("ip netns exec %s tc qdisc add dev %s root %s", name, dev, qdisc)
	return ex.Err()
}

// AddIngressQdisc adds an ingress qdisc.
func (r *Rig) AddRootIngressQdisc(name, dev, qdisc string) error {
	idev := r.idev(dev)
	ex := new(executor.Executor)
	ex.Runf("ip netns exec %s ip link add dev %s type ifb", name, idev)
	ex.Runf("ip netns exec %s tc qdisc add dev %s root handle 1: %s",
		name, idev, qdisc)
	ex.Runf("ip netns exec %s tc qdisc add dev %s handle ffff: ingress",
		name, dev)
	ex.Runf("ip netns exec %s ip link set %s up", name, idev)
	ex.Runf("ip netns exec %s tc filter add dev %s parent ffff: protocol all prio 0 u32 match u32 0 0 flowid 1:1 action mirred egress redirect dev %s",
		name, dev, idev)
	return ex.Err()
}

// Teardown deletes any namespaces in the Rig.
func (r *Rig) Teardown() error {
	r.Lock()
	defer r.Unlock()
	if !r.closed {
		r.closed = true
		close(r.done)
	}

	ex := new(executor.Executor)
	ex.IgnoreErrors = true
	ex.NoLogErrors = true
	for _, name := range r.namespaces {
		ex.Runf("ip netns del %s", name)
	}
	r.namespaces = r.namespaces[:0]
	return ex.Err()
}

func (r *Rig) RightNs(num int) string {
	return fmt.Sprintf("%s%d", r.rightNamePrefix, num)
}

func (r *Rig) MidNs(num int) string {
	return fmt.Sprintf("%s%d", r.midNamePrefix, num)
}

func (r *Rig) LeftNs(num int) string {
	return fmt.Sprintf("%s%d", r.leftNamePrefix, num)
}

func (r *Rig) RightDev(name string) string {
	pfx, num := r.splitName(name)
	if pfx != r.midNamePrefix {
		return fmt.Sprintf("%s.r0", name)
	}
	if num == r.Middleboxes-1 {
		return r.rbdev(name)
	}
	return fmt.Sprintf("%s.r0", name)
}

func (r *Rig) LeftDev(name string) string {
	pfx, num := r.splitName(name)
	if pfx != r.midNamePrefix {
		return fmt.Sprintf("%s.l0", name)
	}
	if num == 0 {
		return r.lbdev(name)
	}
	return fmt.Sprintf("%s.l0", name)
}

func (r *Rig) RightIP(num int) string {
	return fmt.Sprintf("%s%d", rightIPPrefix, num+1)
}

func (r *Rig) RightGatewayIP() string {
	return fmt.Sprintf("%s%d", rightIPPrefix, 254)
}

func (r *Rig) LeftIP(num int) string {
	return fmt.Sprintf("%s%d", leftIPPrefix, num+1)
}

func (r *Rig) LeftGatewayIP() string {
	return fmt.Sprintf("%s%d", leftIPPrefix, 254)
}

func (r *Rig) splitName(name string) (prefix string, num int) {
	panik := func() {
		panic(fmt.Sprintf("invalid rig namespace: %s", name))
	}

	switch {
	case strings.HasPrefix(name, r.leftNamePrefix):
		prefix = r.leftNamePrefix
	case strings.HasPrefix(name, r.midNamePrefix):
		prefix = r.midNamePrefix
	case strings.HasPrefix(name, r.rightNamePrefix):
		prefix = r.rightNamePrefix
	default:
		panik()
	}

	numStr := strings.TrimPrefix(name, prefix)
	var err error
	if num, err = strconv.Atoi(numStr); err != nil {
		panik()
	}

	return
}

func (r *Rig) rdev(name string, num int) string {
	return fmt.Sprintf("%s.r%d", name, num)
}

func (r *Rig) rbdev(name string) string {
	return fmt.Sprintf("%s.rb", name)
}

func (r *Rig) bdev(name string) string {
	return fmt.Sprintf("%s.b", name)
}

func (r *Rig) idev(dev string) string {
	return fmt.Sprintf("i%s", dev)
}

func (r *Rig) ldev(name string, num int) string {
	return fmt.Sprintf("%s.l%d", name, num)
}

func (r *Rig) lbdev(name string) string {
	return fmt.Sprintf("%s.lb", name)
}

func (r *Rig) rightIPNet(num int) string {
	return fmt.Sprintf("%s/%d", r.RightIP(num), netPrefixLen)
}

func (r *Rig) leftIPNet(num int) string {
	return fmt.Sprintf("%s/%d", r.LeftIP(num), netPrefixLen)
}

func (r *Rig) rightGatewayNet() string {
	return fmt.Sprintf("%s/%d", r.RightGatewayIP(), netPrefixLen)
}

func (r *Rig) leftGatewayNet() string {
	return fmt.Sprintf("%s/%d", r.LeftGatewayIP(), netPrefixLen)
}

func (r *Rig) rightNet() string {
	return r.rightIPNet(-1)
}

func (r *Rig) leftNet() string {
	return r.leftIPNet(-1)
}

func (r *Rig) setupRight() error {
	ex := new(executor.Executor)

	for i := 0; i < r.RightEndpoints; i++ {
		name := r.RightNs(i)
		ldev := r.ldev(name, 0)
		lname := r.MidNs(r.Middleboxes - 1)
		lrdev := r.rdev(lname, i)
		ipNet := r.rightIPNet(i)
		r.addNs(name)

		ex.Runf("ip netns add %s", name)
		ex.Runf("ip link add dev %s type veth peer name %s", ldev, lrdev)
		ex.Runf("ip link set dev %s netns %s", ldev, name)
		ex.Runf("ip netns exec %s ip addr add %s dev %s", name, ipNet, ldev)
		ex.Runf("ip netns exec %s ip link set %s up", name, ldev)
		ex.Runf("ip netns exec %s ip route add %s via %s dev %s",
			name, r.leftNet(), r.RightGatewayIP(), ldev)
	}

	return ex.Err()
}

func (r *Rig) setupMid() error {
	ex := new(executor.Executor)

	getRdevs := func(name string, n int) (devs []string) {
		devs = make([]string, n)
		for i := 0; i < n; i++ {
			devs[i] = r.rdev(name, i)
		}
		return
	}

	// set up rightmost middlebox
	setupRightmost := func() {
		num := r.Middleboxes - 1
		name := r.MidNs(num)
		rdevs := getRdevs(name, r.RightEndpoints)
		rbdev := r.rbdev(name)
		rbnet := r.rightGatewayNet()

		// add namespace
		r.addNs(name)
		ex.Runf("ip netns add %s", name)

		// add bridge for right interfaces
		ex.Runf("ip netns exec %s ip link add name %s type bridge",
			name, rbdev)
		ex.Runf("ip netns exec %s ip addr add %s dev %s", name, rbnet, rbdev)
		ex.Runf("ip netns exec %s ip link set dev %s up", name, rbdev)

		// take ownership of and configure right interfaces
		for _, rdev := range rdevs {
			ex.Runf("ip link set dev %s netns %s", rdev, name)
			ex.Runf("ip netns exec %s ip link set %s up", name, rdev)
			ex.Runf("ip netns exec %s ip link set dev %s master %s",
				name, rdev, rbdev)
		}

		// if there are more middleboxes, add a left interface
		if r.Middleboxes > 1 {
			ldev := r.ldev(name, 0)
			lrdev := r.rdev(r.MidNs(num-1), 0)
			ex.Runf("ip link add dev %s type veth peer name %s", ldev, lrdev)
			ex.Runf("ip link set dev %s netns %s", ldev, name)
			ex.Runf("ip netns exec %s ip addr add %s dev %s", name,
				rightBackhaulIPNet, ldev)
			ex.Runf("ip netns exec %s ip link set %s up", name, ldev)
			ex.Runf("ip netns exec %s ip route add %s via %s dev %s",
				name, r.leftNet(), leftBackhaulIP, ldev)
		}

		// enable forwarding
		ex.Runf("ip netns exec %s sysctl -w net.ipv4.ip_forward=1",
			name)
		ex.Runf("ip netns exec %s sysctl -w net.ipv6.conf.all.forwarding=1",
			name)
	}

	// set up intermediary middlebox
	setupIntermediary := func(num int) {
		name := r.MidNs(num)
		ldev := r.ldev(name, 0)
		lrdev := r.rdev(r.MidNs(num-1), 0)
		rdev := r.rdev(name, 0)
		bdev := r.bdev(name)

		// add namespace
		r.addNs(name)
		ex.Runf("ip netns add %s", name)

		// take ownership of and configure right interface
		ex.Runf("ip link set dev %s netns %s", rdev, name)
		ex.Runf("ip netns exec %s ip link set %s up", name, rdev)

		// add left interface
		ex.Runf("ip link add dev %s type veth peer name %s", ldev, lrdev)
		ex.Runf("ip link set dev %s netns %s", ldev, name)
		ex.Runf("ip netns exec %s ip link set %s up", name, ldev)

		// add bridge
		ex.Runf("ip netns exec %s ip link add name %s type bridge", name, bdev)
		ex.Runf("ip netns exec %s ip link set dev %s master %s",
			name, rdev, bdev)
		ex.Runf("ip netns exec %s ip link set dev %s master %s",
			name, ldev, bdev)
		ex.Runf("ip netns exec %s ip link set dev %s up", name, bdev)
	}

	getLdevs := func(name string, n int) (devs []string) {
		devs = make([]string, n)
		for i := 0; i < n; i++ {
			devs[i] = r.ldev(name, i)
		}
		return
	}

	getLRdevs := func() (devs []string) {
		devs = make([]string, r.LeftEndpoints)
		for i := 0; i < r.LeftEndpoints; i++ {
			name := r.LeftNs(i)
			devs[i] = r.rdev(name, 0)
		}
		return
	}

	setupLeftmost := func() {
		num := 0
		name := r.MidNs(num)
		ldevs := getLdevs(name, r.LeftEndpoints)
		lrdevs := getLRdevs()
		lbdev := r.lbdev(name)
		lbnet := r.leftGatewayNet()

		// if needed, add namespace and take ownership of right interfaces
		if r.Middleboxes > 1 {
			rdev := r.rdev(name, 0)
			r.addNs(name)
			ex.Runf("ip netns add %s", name)
			ex.Runf("ip link set dev %s netns %s", rdev, name)
			ex.Runf("ip netns exec %s ip addr add %s dev %s", name,
				leftBackhaulIPNet, rdev)
			ex.Runf("ip netns exec %s ip link set %s up", name, rdev)
			ex.Runf("ip netns exec %s ip route add %s via %s dev %s",
				name, r.rightNet(), rightBackhaulIP, rdev)
		}

		// add bridge for left interfaces
		ex.Runf("ip netns exec %s ip link add name %s type bridge",
			name, lbdev)
		ex.Runf("ip netns exec %s ip addr add %s dev %s", name, lbnet, lbdev)
		ex.Runf("ip netns exec %s ip link set dev %s up", name, lbdev)

		// add left interfaces
		for j, ldev := range ldevs {
			ex.Runf("ip link add dev %s type veth peer name %s",
				ldev, lrdevs[j])
			ex.Runf("ip link set dev %s netns %s", ldev, name)
			ex.Runf("ip netns exec %s ip link set %s up", name, ldev)
			ex.Runf("ip netns exec %s ip link set dev %s master %s",
				name, ldev, lbdev)
		}

		// enable forwarding
		ex.Runf("ip netns exec %s sysctl -w net.ipv4.ip_forward=1", name)
		ex.Runf("ip netns exec %s sysctl -w net.ipv6.conf.all.forwarding=1",
			name)
	}

	// set up rightmost, intermediary, then leftmost middleboxes
	setupRightmost()

	for i := r.Middleboxes - 2; i > 0; i-- {
		setupIntermediary(i)
	}

	setupLeftmost()

	return ex.Err()
}

func (r *Rig) setupLeft() error {
	ex := new(executor.Executor)

	for i := 0; i < r.LeftEndpoints; i++ {
		name := r.LeftNs(i)
		rdev := r.rdev(name, 0)
		ipNet := r.leftIPNet(i)
		r.addNs(name)

		ex.Runf("ip netns add %s", name)
		ex.Runf("ip link set dev %s netns %s", rdev, name)
		ex.Runf("ip netns exec %s ip addr add %s dev %s", name, ipNet, rdev)
		ex.Runf("ip netns exec %s ip link set %s up", name, rdev)
		ex.Runf("ip netns exec %s ip route add %s via %s dev %s",
			name, r.rightNet(), r.LeftGatewayIP(), rdev)
	}

	return ex.Err()
}

func (r *Rig) addNs(name string) {
	r.namespaces = append(r.namespaces, name)
}

func randomPrefix() string {
	r := rand.New(rand.NewSource(time.Now().UTC().UnixNano()))
	b := make([]byte, randomPrefixLen)
	for i := range b {
		b[i] = alphaNum[r.Intn(len(alphaNum))]
	}
	return string(b)
}
