package metrics

import (
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

// KeyMethod is used to identified the method stats are collected for.
var KeyMethod, _ = tag.NewKey("method")

// Opencensus observables
var (
	// ProcessBlock duration in milliseconds
	MProcessBlockMs = stats.Float64("consensus/process_block", "The duration in milliseconds of ProcessBlock", stats.UnitMilliseconds)

	processBlockView = &view.View{
		Name:        "consensus/processor",
		Measure:     MProcessBlockMs,
		Description: "The distribution of the durations",

		// Latency in buckets:
		// [>=0ms, >=25ms, >=50ms, >=75ms, >=100ms, >=200ms, >=400ms, >=600ms, >=800ms, >=1s, >=2s, >=4s, >=8s]
		Aggregation: view.Distribution(25, 50, 75, 100, 200, 400, 600, 800, 1000, 2000, 4000, 8000),
		TagKeys:     []tag.Key{KeyMethod}}
)
