package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"time"
)

// --------- Input model ---------
type Player struct {
	Name           string    `json:"name"`
	Differentials  []float32 `json:"differentials"` // Most-recent first
	EventsPlayed   int       `json:"eventsPlayed"`
	PointsPerEvent float32   `json:"pointsPerEvent"`
	HandicapIndex  float32   `json:"handicapIndex"` // Optional; estimated if zero
}

// --------- Parameters (flags) ---------
var (
	sims              = flag.Int("sims", 50000, "number of Monte-Carlo iterations")
	urlFlag           = flag.String("url", "", "leaderboard URL, e.g. https://…/leaderboard.htm")
	pointsWeight      = flag.Float64("wLeague", 0.40, "weight on league form (0-1)")
	decay             = flag.Float64("decay", 0.90, "exponential decay for recent rounds (0-1)")
	handicapAllowance = flag.Float64("allowance", 0.90, "handicap allowance (0-1)")
)

// --------- Helpers ---------

// Weighted average of recent net differentials using exponential decay
func expAvg(diffs []float64, decay float64) float64 {
	var num, den float64
	w := 1.0
	for _, d := range diffs {
		num += d * w
		den += w
		w *= decay
	}
	if den == 0 {
		return 0
	}
	return num / den
}

// Compute mean and standard deviation
func meanStd(vals []float64) (mean, sd float64) {
	for _, v := range vals {
		mean += v
	}
	mean /= float64(len(vals))
	for _, v := range vals {
		sd += (v - mean) * (v - mean)
	}
	sd = math.Sqrt(sd / float64(len(vals)))
	if sd == 0 {
		sd = 1
	}
	return
}

// Convert win probability to rounded money-line odds
func probToMoneyline(p float64) int {
	if p <= 0 || p >= 1 {
		return 0
	}
	if p >= 0.5 {
		raw := -p / (1 - p) * 100
		return int(math.Round(raw/10)) * 10
	}
	raw := (1 - p) / p * 100
	if raw < 200 {
		return int(math.Ceil(raw/10)) * 10
	}
	return int(math.Ceil(raw/25)) * 25
}

// --------- Output model ---------
type Result struct {
	Name      string
	Prob      float64
	MoneyLine int
}

// --------- Core logic ---------

// MonteCarloOdds with Laplace smoothing and points shrinkage.
func CalculateOdds(players []*Player, wPts, decay, handicapAllowance float64, sims int) []Result {
	const (
		minSD = 1.5 // strokes – floor for volatility
		tau   = 3.0 // shrinkage strength for PointsPerEvent
		alpha = 1   // Laplace pseudocount
	)
	// ---------- pool players with a handicap ----------
	var pool []*Player
	for _, p := range players {
		if len(p.Differentials) > 0 {
			pool = append(pool, p)
		}
	}
	if len(pool) == 0 {
		return nil
	}

	// ---------- compute field mean PPE (for shrink) ----------
	var sumPts, sumSq float64
	var cnt float64
	for _, p := range pool {
		if p.EventsPlayed > 0 {
			v := float64(p.PointsPerEvent)
			sumPts += v
			sumSq += v * v
			cnt++
		}
	}
	meanPPE, sdPPE := 0.0, 1.0
	if cnt > 0 {
		meanPPE = sumPts / cnt
		varVar := (sumSq/cnt - meanPPE*meanPPE)
		if varVar > 0 {
			sdPPE = math.Sqrt(varVar)
		}
	}

	// ---------- per-player stats ----------
	type stats struct{ mu, sd float64 }
	ps := make([]stats, len(pool))

	for i, p := range pool {
		idxAdj := handicapAllowance * float64(p.HandicapIndex)

		net := make([]float64, len(p.Differentials))
		for k, d := range p.Differentials {
			net[k] = float64(d) - idxAdj
		}

		mu := expAvg(net, decay)
		_, sd := meanStd(net)
		if sd < minSD {
			sd = minSD
		}

		// --- shrink PointsPerEvent toward mean ---
		if p.EventsPlayed > 0 {
			n := float64(p.EventsPlayed)
			shrunk := (n/(n+tau))*float64(p.PointsPerEvent) + (tau/(n+tau))*meanPPE
			zPts := (shrunk - meanPPE) / sdPPE
			mu += wPts * (-zPts) // strokes per σ of league form
		}
		ps[i] = stats{mu: mu, sd: sd}
	}

	// ---------- Monte-Carlo ----------
	winCnt := make([]int, len(pool))
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for n := 0; n < sims; n++ {
		best := math.MaxFloat64
		winner := -1

		for i, st := range ps {
			score := rng.NormFloat64()*st.sd + st.mu
			if score < best {
				best, winner = score, i
			} else if score == best && rng.Intn(2) == 0 { // coin-flip tie
				winner = i
			}
		}
		winCnt[winner]++
	}

	// ---------- convert to probabilities with Laplace smoothing ----------
	results := make([]Result, len(pool))
	total := float64(sims + alpha*len(pool))

	for i, p := range pool {
		prob := float64(winCnt[i]+alpha) / total
		results[i] = Result{
			Name:      p.Name,
			Prob:      prob,
			MoneyLine: probToMoneyline(prob),
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Prob > results[j].Prob })
	return results
}

// --------- Main entry point ---------

func main() {
	flag.Parse()

	var players []*Player
	var err error

	switch {
	case *urlFlag != "": // --- scrape live site -------------------------
		players, err = CreateField(*urlFlag)
		if err != nil {
			log.Fatalf("scrape failed: %v", err)
		}

	case stdinHasData(): // --- read players from stdin ------------------
		if err := json.NewDecoder(os.Stdin).Decode(&players); err != nil {
			log.Fatalf("invalid JSON: %v", err)
		}

	default: // --- no input given ---------------------------------------
		log.Fatalf("please supply -url or pipe player JSON to stdin")
	}

	results := CalculateOdds(players, *pointsWeight, *decay, *handicapAllowance, *sims)

	fmt.Printf("%-12s %6s %8s\n", "Player", "Prob%", "Odds")
	for _, r := range results {
		sign := "+"
		if r.MoneyLine < 0 {
			sign = ""
		}
		fmt.Printf("%-12s %5.2f%%  %s%d\n", r.Name, r.Prob*100, sign, r.MoneyLine)
	}
}

func stdinHasData() bool {
	stat, _ := os.Stdin.Stat()
	return (stat.Mode() & os.ModeCharDevice) == 0
}
