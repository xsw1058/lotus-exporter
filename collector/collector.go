package collectors

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xsw1058/lotus-exporter/metrics"
)

//var (
// scrapeSuccessDesc = prometheus.NewDesc(
//
//	prometheus.BuildFQName(namespace, "scrape", "collector_success"),
//	"lotus_exporter: Whether a collector succeeded.",
//	[]string{"collector"},
//	nil,
//
// )
// scrapeFailedDesc = prometheus.NewDesc(
//
//	prometheus.BuildFQName(namespace, "scrape", "collector_failed"),
//	"lotus_exporter: Whether a collector failed.",
//	[]string{"collector"},
//	nil,
//
// )
//)

//var (
//	//initiatedCollectorsMtx = sync.Mutex{}
//	factories = make(map[string]func(loader metrics.Loader) LotusCollector)
//)

//func RegisterCollector(collectorName string, factory func(loader metrics.Loader) LotusCollector) {
//	factories[collectorName] = factory
//}

type AllCollector struct {
	collectors map[string]metrics.LotusCollector
	//rightNowUpdateResult   []metrics.UpdateResult
	scrapeAtDesc           *prometheus.GaugeVec
	scrapeDurationDesc     *prometheus.GaugeVec
	scrapeLatestStatusDesc *prometheus.GaugeVec
	res                    []metrics.UpdateResult
	//storage                metrics.Loader
}

//func contains(elems []string, v string) bool {
//	for _, s := range elems {
//		if v == s {
//			return true
//		}
//	}
//	return false
//}

func NewAllCollector(u metrics.UpdateResp) prometheus.Collector {

	// 1. 先发送更新请求
	cs := make(map[string]metrics.LotusCollector)

	//var rightNowUpdate = metrics.RightNowUpdate
	//ch := u.HandlerUpdate(rightNowUpdate)
	//var r []metrics.UpdateResult
	//for res := range ch {
	//	if !contains(rightNowUpdate, res.Handler) {
	//		continue
	//	}
	//	r = append(r, res)
	//}
	//
	////  TODO 单独自动构建.
	//// 2. 再构建collector
	//cs[metrics.DemoKey] = NewDemoCollector(storage)
	//cs[metrics.Demo2Key] = NewDemo2Collector(storage)
	//cs[metrics.Demo3Key] = NewDemo3Collector(storage)
	//cs[metrics.InfoKey] = NewInfoCollector(storage)
	var a AllCollector
	var res []metrics.UpdateResult
	ch := u.Aggregation()
	for aa := range ch {
		res = append(res, aa.Res)
		if aa.Res.Success {
			cs[aa.Res.Handler] = aa.Builder
		}
	}

	a.scrapeAtDesc = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metrics.NameSpace,
			Name:      "collector_start_at",
			Help:      "collector_start_at.",
		}, []string{"collector"})

	a.scrapeDurationDesc = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metrics.NameSpace,
			Name:      "collector_duration_seconds",
			Help:      "collector_duration_seconds.",
		}, []string{"collector"})

	a.scrapeLatestStatusDesc = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metrics.NameSpace,
			Name:      "collector_latest_status",
			Help:      "collector_latest_status.",
		}, []string{"collector"})

	a.collectors = cs
	a.res = res
	//a.rightNowUpdateResult = r
	//a.storage = storage
	return &a
}
func (a *AllCollector) Describe(ch chan<- *prometheus.Desc) {
}

func (a *AllCollector) Collect(ch chan<- prometheus.Metric) {
	a.scrapeDurationDesc.Reset()
	a.scrapeAtDesc.Reset()
	a.scrapeLatestStatusDesc.Reset()

	//var allResult []metrics.UpdateResult
	//value := a.storage.Load(metrics.ResultKey)
	//r, ok := value.([]metrics.UpdateResult)
	//if ok {
	//	allResult = append(allResult, r...)
	//}
	//allResult = append(allResult, a.rightNowUpdateResult...)

	for _, c := range a.collectors {
		if c == nil {
			//log.Printf("%v is nil, pass", n)
			continue
		}
		c.Update(ch)
	}

	for _, res := range a.res {
		if res.Success {
			a.scrapeLatestStatusDesc.WithLabelValues(res.Handler).Set(1)
		} else {
			a.scrapeLatestStatusDesc.WithLabelValues(res.Handler).Set(0)
		}
		a.scrapeAtDesc.WithLabelValues(res.Handler).Set(float64(res.StartAt.Unix()))
		a.scrapeDurationDesc.WithLabelValues(res.Handler).Set(res.Duration.Seconds())
	}
	a.scrapeLatestStatusDesc.Collect(ch)
	a.scrapeDurationDesc.Collect(ch)
	a.scrapeAtDesc.Collect(ch)

}
