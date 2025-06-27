package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
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
	urlFlag      = flag.String("url", "", "leaderboard URL, e.g. https://…/leaderboard.htm")
	recWeight    = flag.Float64("wRecency", 0.60, "weight on recent form (0-1)")
	pointsWeight = flag.Float64("wLeague", 0.40, "weight on league form (0-1)")
	decay        = flag.Float64("decay", 0.90, "exponential decay for recent rounds (0-1)")
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

// Estimate handicap index using USGA method: best 8 of last 20 × 0.96
func estimateIndex(diffs []float32) float64 {
	n := len(diffs)
	if n == 0 {
		return 0
	}
	m := 8
	if n < 8 {
		m = n
	}
	sorted := make([]float32, n)
	copy(sorted, diffs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum float64
	for i := 0; i < m; i++ {
		sum += float64(sorted[i])
	}
	return 0.96 * sum / float64(m)
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

func CalculateOdds(players []*Player, recWeight, pointsWeight, decay float64) []Result {
	var pool []*Player
	for _, p := range players {
		if p.HandicapIndex == 0 {
			continue // skip – insufficient data
		}
		pool = append(pool, p)
	}
	if len(pool) == 0 {
		return nil // nothing to rate
	}

	n := len(pool)
	recent := make([]float64, n)
	points := make([]float64, n)

	for i, p := range pool {

		// Estimate or use provided handicap index
		idx := float64(p.HandicapIndex)
		if idx == 0 {
			idx = estimateIndex(p.Differentials)
		}

		// Calculate net differentials (d - index)
		netDiffs := make([]float64, len(p.Differentials))
		for j, d := range p.Differentials {
			netDiffs[j] = float64(d) - idx
		}

		// Recency-weighted average of net performance
		recent[i] = expAvg(netDiffs, decay)

		// Invert points: higher points = better player = lower score
		points[i] = -float64(p.PointsPerEvent)
	}

	// Z-score normalize
	meanR, sdR := meanStd(recent)
	meanP, sdP := meanStd(points)

	z := make([]float64, n)
	for i := 0; i < n; i++ {
		zRec := (recent[i] - meanR) / sdR
		zPts := (points[i] - meanP) / sdP
		z[i] = recWeight*zRec + pointsWeight*zPts
	}

	// Softmax to convert to probabilities
	expVals := make([]float64, n)
	var sumExp float64
	for i, v := range z {
		expVals[i] = math.Exp(-v)
		sumExp += expVals[i]
	}

	// Final output
	results := make([]Result, n)
	for i, p := range pool {
		prob := expVals[i] / sumExp
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

	results := CalculateOdds(players, *recWeight, *pointsWeight, *decay)

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
