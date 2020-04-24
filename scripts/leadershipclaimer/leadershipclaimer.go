// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/leadership"
	"github.com/juju/juju/apiserver/params"
	coreleadership "github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
)

var unit = gnuflag.String("unit", "ubuntu-lite/0", "set the unit name that we will connect as")
var password = gnuflag.String("password", "", "the password for this agent")
var hosts = gnuflag.String("hosts", "localhost", "the hosts to connect to (comma separated)")
var port = gnuflag.Int("port", 17070, "the apiserver port")
var uuid = gnuflag.String("uuid", "", "model-uuid to connect to")
var claimtime = gnuflag.String("claimtime", "10s", "time that we will request to hold the lease")
var renewtime = gnuflag.String("renewtime", "", "how often we will renew the lease (default 1/2 the claim time)")
var quiet = gnuflag.Bool("quiet", false, "print only when the leases are claimed")
var initSleep = gnuflag.String("sleep", "1s", "time to sleep before starting processing")

var agentStart time.Time

func main() {
	loggo.GetLogger("").SetLogLevel(loggo.INFO)
	start := time.Now()
	rand.Seed(int64(start.Nanosecond() + os.Getpid()))
	gnuflag.Parse(true)
	// make it a little bit easier to have all of the processes start closer to the same time.
	// don't start doing any real work for the first second.
	sleepDuration, err := time.ParseDuration(*initSleep)
	if err != nil {
		panic(err)
	}
	time.Sleep(sleepDuration)
	claimDuration, err := time.ParseDuration(*claimtime)
	if err != nil {
		panic(err)
	}
	renewDuration := claimDuration / 2
	if *renewtime != "" {
		renewDuration, err = time.ParseDuration(*renewtime)
		if err != nil {
			panic(err)
		}
	}
	if !names.IsValidUnit(*unit) {
		panic(fmt.Sprintf("must supply a valid unit name, not: %q", *unit))
	}
	modelTag := names.NewModelTag(*uuid)
	unitTag := names.NewUnitTag(*unit)
	if err != nil {
		panic(err)
	}
	holders := gnuflag.Args()
	if len(holders) == 0 {
		holders = []string{*unit}
	}
	holderTags := make([]names.UnitTag, len(holders))
	for i := range holders {
		holderTags[i] = names.NewUnitTag(holders[i])
	}
	hostNames := strings.Split(*hosts, ",")
	infos := make([]*api.Info, len(hostNames))
	for i := range hostNames {
		info := &api.Info{
			Addrs:    []string{net.JoinHostPort(hostNames[i], fmt.Sprint(*port))},
			ModelTag: modelTag,
			Tag:      unitTag,
			Password: *password,
		}
		infos[i] = info
	}
	agentStart = time.Now()
	var wg sync.WaitGroup
	for htCount, holderTag := range holderTags {
		hostCounter := holderTag.Number() % len(infos)
		info := infos[hostCounter]
		var conn api.Connection
		delay := time.Second
		for i := 0; i < 5; i++ {
			var err error
			start := time.Now()
			conn, err = connect(info)
			sinceStart := time.Since(agentStart).Round(time.Millisecond).Seconds()
			fmt.Fprintf(os.Stdout, "%9.3fs connected [%6d] %4d %s in %s\n",
				sinceStart, os.Getpid(), htCount, holderTag.Id(), time.Since(start).Round(time.Millisecond))
			if err == nil {
				break
			} else {
				if strings.Contains(strings.ToLower(err.Error()), "try again") {
					fmt.Fprintf(os.Stderr, "%d failed to connect to %v for %v (retrying): %v\n", htCount, info, holderTag.Id(), err)
					if i < 4 {
						time.Sleep(delay)
						delay *= 2
					}
					continue
				}
				// fmt.Fprintf(os.Stderr, "failed to connect to %v for %v: %v\n", info, holderTag.Id(), err)
				fmt.Fprintf(os.Stdout, "%d failed to connect to %v for %v: %v\n", htCount, info, holderTag.Id(), err)
			}
		}
		if conn == nil {
			continue
		}
		wg.Add(1)
		go func(tag names.UnitTag, conn api.Connection) {
			defer wg.Done()
			defer conn.Close()
			claimLoop(tag, leadership.NewClient(conn), claimDuration, renewDuration)
		}(holderTag, conn)
	}
	wg.Wait()
}

func connect(info *api.Info) (api.Connection, error) {
	opts := api.DefaultDialOpts()
	opts.InsecureSkipVerify = true
	conn, err := api.Open(info, opts)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func leaderSet(facadeCaller base.FacadeCaller, holderTag names.UnitTag, keys map[string]string) error {
	appId, err := names.UnitApplication(holderTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	applicationTag := names.NewApplicationTag(appId)
	args := params.MergeLeadershipSettingsBulkParams{
		Params: []params.MergeLeadershipSettingsParam{{
			ApplicationTag: applicationTag.String(),
			UnitTag:        holderTag.String(),
			Settings:       keys,
		}},
	}
	var results params.ErrorResults
	err = facadeCaller.FacadeCall("Merge", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	err = results.OneError()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func claimLoop(holderTag names.UnitTag, claimer coreleadership.Claimer, claimDuration, renewDuration time.Duration) {
	next := time.After(0)
	leaseName, err := names.UnitApplication(holderTag.Id())
	if err != nil {
		panic(err)
	}
	isLeader := false
	var isLeaderTime time.Time
	for {
		select {
		case <-next:
			start := time.Now()
			err := claimer.ClaimLeadership(leaseName, holderTag.Id(), claimDuration)
			now := time.Now()
			sinceStart := now.Sub(agentStart).Round(time.Millisecond).Seconds()
			reqDuration := now.Sub(start).Round(time.Millisecond)
			if err == nil {
				next = time.After(renewDuration)
				if isLeader {
					heldFor := now.Sub(isLeaderTime).Round(time.Second)
					if *quiet {
						fmt.Fprintf(os.Stdout, "%9.3fs extended %s held for %s in %s\n",
							sinceStart, holderTag.Id(), heldFor, reqDuration)
					} else {
						fmt.Fprintf(os.Stdout, "%9.3fs extended leadership of %q for %q for %v in %s, held for %s, renewing after %v\n",
							sinceStart, leaseName, holderTag.Id(), claimDuration, reqDuration, heldFor, renewDuration)
					}
				} else {
					if *quiet {
						fmt.Fprintf(os.Stdout, "%9.3fs claimed  %s in %s\n", sinceStart, holderTag.Id(), reqDuration)
					} else {
						fmt.Fprintf(os.Stdout, "%9.3fs claimed leadership of %q for %q for %v in %s, renewing after %v\n",
							sinceStart, leaseName, holderTag.Id(), claimDuration, reqDuration, renewDuration)
					}
					isLeaderTime = time.Now()
				}
				isLeader = true
			} else {
				if errors.Cause(err) == coreleadership.ErrClaimDenied {
					now := time.Now()
					sinceStart := now.Sub(agentStart).Round(time.Millisecond).Seconds()
					if isLeader {
						heldFor := now.Sub(isLeaderTime).Round(time.Second)
						if *quiet {
							fmt.Fprintf(os.Stdout, "%9.3fs lost     %s after %s in %s\n",
								sinceStart, holderTag.Id(), heldFor, reqDuration)
						} else {
							fmt.Fprintf(os.Stdout, "%9.3fs lost leadership of %q for %q after %s in %s, blocking until released\n",
								sinceStart, leaseName, holderTag.Id(), heldFor, reqDuration)
						}
					} else {
						if !*quiet {
							fmt.Fprintf(os.Stdout, "%9.3fs claim of %q for %q denied in %s, blocking until released\n",
								sinceStart, leaseName, holderTag.Id(), reqDuration)
						}
					}
					isLeader = false
					isLeaderTime = time.Time{}
					// Note: the 'cancel' channel does nothing
					start := now
					err := claimer.BlockUntilLeadershipReleased(leaseName, nil)
					now = time.Now()
					sinceStart = now.Sub(agentStart).Round(time.Millisecond).Seconds()
					reqDuration := now.Sub(start).Round(time.Millisecond)
					if err != nil {
						fmt.Fprintf(os.Stderr, "%9.3fs blocking for leadership of %q for %q failed in %s with %v\n",
							sinceStart, leaseName, holderTag.Id(), reqDuration, err)
						return
					}
					if !*quiet {
						fmt.Fprintf(os.Stdout, "%9.3fs blocking of %q for %q returned after %s, attempting to claim\n",
							sinceStart, leaseName, holderTag.Id(), reqDuration)
					}
					next = time.After(0)
				} else if errors.Cause(err) == lease.ErrTimeout {
					fmt.Fprintf(os.Stderr, "%9.3fs claim of %q for %q timed out in %s, retrying\n",
						sinceStart, leaseName, holderTag.Id(), reqDuration)
					fmt.Fprintf(os.Stdout, "%9.3fs claim of %q for %q timed out in %s, retrying\n",
						sinceStart, leaseName, holderTag.Id(), reqDuration)
				} else {
					fmt.Fprintf(os.Stderr, "%9.3fs claim of %q for %q failed in %s: %v\n",
						sinceStart, leaseName, holderTag.Id(), reqDuration, err)
					return
				}
			}
		}
	}
}
