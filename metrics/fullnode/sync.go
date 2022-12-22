package fullnode

import (
	"errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xsw1058/lotus-exporter/metrics"
	"strconv"
)

type SyncMetrics struct {
	Status string
	Diff   float64
}

func (l *FullNode) ChainSync() ([]SyncMetrics, error) {
	var s []SyncMetrics
	syncState, err := l.api.SyncState(l.ctx)
	if err != nil {
		return nil, err
	}
	for _, ss := range syncState.ActiveSyncs {
		var heightDiff int64
		if ss.Base != nil {
			heightDiff = int64(ss.Base.Height())
		}
		if ss.Target != nil {
			heightDiff = int64(ss.Target.Height()) - heightDiff
		} else {
			heightDiff = 0
		}
		s = append(s, SyncMetrics{
			Status: ss.Stage.String(),
			Diff:   float64(heightDiff),
		})
	}
	return s, nil
}

type Sync struct {
	syncStatus *prometheus.GaugeVec
	native     []SyncMetrics
}

func NewSyncCollector(data []SyncMetrics) (metrics.LotusCollector, error) {
	if len(data) == 0 {
		return nil, errors.New("no data")
	}
	return &Sync{
		syncStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "sync",
				Help:      "sync.",
			}, []string{"worker_id", "status"}),
		native: data,
	}, nil
}
func (s *Sync) Update(ch chan<- prometheus.Metric) {
	s.syncStatus.Reset()
	for i, sync := range s.native {
		s.syncStatus.WithLabelValues(strconv.Itoa(i), sync.Status).Set(sync.Diff)
	}
	s.syncStatus.Collect(ch)
}
