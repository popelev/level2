package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	SamplesWritten = promauto.NewCounter(prometheus.CounterOpts{
		Name: "level2_samples_written_total",
		Help: "Samples successfully written to historian",
	})
	SamplesSpooled = promauto.NewCounter(prometheus.CounterOpts{
		Name: "level2_samples_spooled_total",
		Help: "Samples written to disk spool after historian failure",
	})
	WriteErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "level2_historian_write_errors_total",
		Help: "Historian write failures",
	})
	OPCConnected = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "level2_opc_connected",
		Help: "1 if at least one OPC driver is connected",
	})
	SpoolDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "level2_spool_depth",
		Help: "Number of spool files waiting for replay",
	})
	CapacityHalts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "level2_historian_capacity_halts_total",
		Help: "Write batches skipped because DB capacity policy halted writes",
	})
	CapacityDrops = promauto.NewCounter(prometheus.CounterOpts{
		Name: "level2_historian_capacity_drops_total",
		Help: "Oldest-data drop operations performed under drop_oldest policy",
	})
)

func Handler() http.Handler { return promhttp.Handler() }
