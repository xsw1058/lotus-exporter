package collector

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xsw1058/lotus-exporter/metrics"
)

type AllCollector struct {
	collectors             map[string]metrics.LotusCollector
	scrapeAtDesc           *prometheus.GaugeVec
	scrapeDurationDesc     *prometheus.GaugeVec
	scrapeLatestStatusDesc *prometheus.GaugeVec
	res                    []metrics.ScrapeStatus
}

const NameSpace = "collector"

func NewAllCollector(u metrics.AggregationUpdater) prometheus.Collector {

	cs := make(map[string]metrics.LotusCollector)

	var a AllCollector
	var res []metrics.ScrapeStatus
	ch := u.Aggregation()
	for aa := range ch {
		res = append(res, aa.Status)
		if aa.Status.Success {
			cs[aa.Status.HandlerKey] = aa.Builder
		}
	}

	a.scrapeAtDesc = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      "start_at",
			Help:      "collector_start_at.",
		}, []string{"collector"})

	a.scrapeDurationDesc = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      "duration_seconds",
			Help:      "collector_duration_seconds.",
		}, []string{"collector"})

	a.scrapeLatestStatusDesc = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      "latest_status",
			Help:      "collector_latest_status.",
		}, []string{"collector"})

	a.collectors = cs
	a.res = res

	return &a
}
func (a *AllCollector) Describe(ch chan<- *prometheus.Desc) {
}

func (a *AllCollector) Collect(ch chan<- prometheus.Metric) {
	a.scrapeDurationDesc.Reset()
	a.scrapeAtDesc.Reset()
	a.scrapeLatestStatusDesc.Reset()

	for _, c := range a.collectors {
		if c == nil {
			continue
		}
		c.Update(ch)
	}

	for _, res := range a.res {
		if res.Success {
			a.scrapeLatestStatusDesc.WithLabelValues(res.HandlerKey).Set(1)
		} else {
			a.scrapeLatestStatusDesc.WithLabelValues(res.HandlerKey).Set(0)
		}
		a.scrapeAtDesc.WithLabelValues(res.HandlerKey).Set(float64(res.StartAt.Unix()))
		a.scrapeDurationDesc.WithLabelValues(res.HandlerKey).Set(res.Duration.Seconds())
	}
	a.scrapeLatestStatusDesc.Collect(ch)
	a.scrapeDurationDesc.Collect(ch)
	a.scrapeAtDesc.Collect(ch)

}
