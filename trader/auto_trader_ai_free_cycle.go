package trader

import (
	"nofx/kernel"
	"sort"
	"strings"
	"time"
)

const aiFreeCycleCloseGrace = 3 * time.Second

func next15mCloseDelay(now time.Time) time.Duration {
	base := now.Truncate(15 * time.Minute)
	next := base.Add(15*time.Minute + aiFreeCycleCloseGrace)
	if !next.After(now) { next = next.Add(15 * time.Minute) }
	return next.Sub(now)
}

func (at *AutoTrader) runAIFreeAlignedLoop() error {
	for {
		at.isRunningMutex.RLock()
		running := at.isRunning
		at.isRunningMutex.RUnlock()
		if !running { return nil }

		timer := time.NewTimer(next15mCloseDelay(time.Now()))
		select {
		case <-timer.C:
			if err := at.runCycle(); err != nil { at.logErrorf("❌ ai_free cycle failed: %v", err) }
		case <-at.stopMonitorCh:
			if !timer.Stop() {
				select { case <-timer.C: default: }
			}
			return nil
		}
	}
}

func sortAIFreeDecisions(in []kernel.Decision) []kernel.Decision {
	closed := map[string]bool{}
	for _, d := range in {
		if d.Action == "close_long" || d.Action == "close_short" { closed[strings.ToUpper(d.Symbol)] = true }
	}
	out := make([]kernel.Decision,0,len(in))
	for _, d := range in {
		if (d.Action=="open_long" || d.Action=="open_short") && closed[strings.ToUpper(d.Symbol)] { continue }
		out = append(out,d)
	}
	p := func(a string) int {
		switch a {
		case "close_long","close_short": return 0
		case "open_long","open_short": return 1
		default: return 2
		}
	}
	sort.SliceStable(out, func(i,j int) bool {
		pi,pj:=p(out[i].Action),p(out[j].Action)
		if pi!=pj { return pi<pj }
		if pi==1 && out[i].Confidence!=out[j].Confidence { return out[i].Confidence>out[j].Confidence }
		return false
	})
	return out
}
