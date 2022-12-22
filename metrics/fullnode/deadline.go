package fullnode

import (
	"errors"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xsw1058/lotus-exporter/metrics"
	"strconv"
	"sync"
)

type Deadline struct {
	Index         string  // deadline ID
	Partitions    float64 // 分区数
	Sectors       float64 // 总扇区数
	ActiveSectors float64 // 活动扇区数
	Faults        float64 // 错误扇区数
	Recovery      float64 // 恢复扇区数
	Proven        float64 // proven partition
	OpenTime      float64 // 开始时间
	OpenEpoch     float64 // 开始高度
}
type DeadlinesMetrics struct {
	ActorStr         string // 矿工号
	ActorTag         string
	CurrentIdx       string     // 当前deadline ID
	PeriodStart      string     // 当前周期开始的epoch
	CurrentOpenEpoch string     // 当前窗口的挑战字epoch
	CurrentCost      float64    // 只计算当前窗口的耗时
	DlsMetrics       []Deadline // 每个窗口的信息
}

func (l *FullNode) minerDeadlines(actor address.Address, tag string) (*DeadlinesMetrics, error) {
	actorStr := actor.String()
	di, err := l.api.StateMinerProvingDeadline(l.ctx, actor, l.chain.chainHead.Key())
	if err != nil {
		return nil, err
	}
	dls, err := l.api.StateMinerDeadlines(l.ctx, actor, l.chain.chainHead.Key())
	if err != nil {
		return nil, err
	}

	d := DeadlinesMetrics{
		ActorStr:         actorStr,
		ActorTag:         tag,
		CurrentIdx:       strconv.Itoa(int(di.Index)),
		PeriodStart:      di.PeriodStart.String(),
		CurrentOpenEpoch: di.Open.String(),
		CurrentCost:      float64(0),
		DlsMetrics:       nil,
	}

	for dlIdx, dl := range dls {
		partitions, err := l.api.StateMinerPartitions(l.ctx, actor, uint64(dlIdx), l.chain.chainHead.Key())
		if err != nil {
			return nil, err
		}

		// 忽略没有分区的窗口。
		if len(partitions) < 1 {
			if di.Index == uint64(dlIdx) {
				d.CurrentCost = -1
			}
			continue
		}
		openEpoch := float64(di.PeriodStart) + (float64(dlIdx) * 60)
		openTime := (openEpoch-l.chain.baseEpoch)*30 + l.chain.baseTimestamp

		provenPartitions, err := dl.PostSubmissions.Count()
		if err != nil {
			return nil, err
		}
		sectors := float64(0)
		actives := float64(0)
		faults := float64(0)
		recovery := float64(0)

		haveActiveSectorPartitions := uint64(0)

		for _, partition := range partitions {
			sc, err := partition.AllSectors.Count()
			if err != nil {
				log.Warnf("getting %s AllSectors: %s", actorStr, err)
				continue
			}
			sectors += float64(sc)

			active, err := partition.ActiveSectors.Count()
			if err != nil {
				log.Warnf("getting %s ActiveSectors: %s", actorStr, err)
				continue
			}
			actives += float64(active)
			if actives > 0 {
				haveActiveSectorPartitions += 1
			}
			fc, err := partition.FaultySectors.Count()
			if err != nil {
				log.Warnf("getting %s FaultySectors: %s", actorStr, err)
				continue
			}
			faults += float64(fc)

			rec, err := partition.RecoveringSectors.Count()
			if err != nil {
				log.Warnf("getting %s RecoveringSectors: %s", actorStr, err)
				continue
			}
			recovery += float64(rec)
		}

		// 只计算当前窗口的耗时。
		if di.Index == uint64(dlIdx) {

			// 有活动扇区的分区数大于提交证明的分区数 表示还没有做完证明
			if haveActiveSectorPartitions > provenPartitions {
				d.CurrentCost = float64(di.CurrentEpoch) - openEpoch
				//log.Debugw("proven cost", "miner", actorStr, "index", dlIdx, "cost", d.CurrentCost)
			} else {
				// 总分区数 == 提交证明的分区数
				if uint64(len(partitions)) == provenPartitions {
					d.CurrentCost = -1
					// 总分区数 > 提交证明的分区数
				} else if uint64(len(partitions)) > provenPartitions {
					d.CurrentCost = -2
					// 其他情况
				} else {
					log.Warnw("proven cost", "miner", actorStr, "index", dlIdx,
						"partitions", partitions,
						"provenPartitions", provenPartitions,
						"haveActiveSectorPartitions", haveActiveSectorPartitions)
					d.CurrentCost = -3
				}
			}
		}
		d.DlsMetrics = append(d.DlsMetrics, Deadline{
			Index:         strconv.Itoa(dlIdx),
			Partitions:    float64(len(partitions)),
			Sectors:       sectors,
			ActiveSectors: actives,
			Faults:        faults,
			Recovery:      recovery,
			Proven:        float64(provenPartitions),
			OpenTime:      openTime,
			OpenEpoch:     openEpoch,
		})
	}
	return &d, nil
}

func (l *FullNode) UpdateDeadlines() ([]*DeadlinesMetrics, error) {

	var data []*DeadlinesMetrics
	var miners = make(map[address.Address]string)
	for _, m := range l.chain.attentionActor {
		if m.ActorType == actors.MinerKey {
			miners[m.ActorID] = m.Tag
		}
	}

	wg := sync.WaitGroup{}
	wg.Add(len(miners))
	for m, tag := range miners {
		go func(actor address.Address, tag string) {
			defer wg.Done()

			deadlines, err := l.minerDeadlines(actor, tag)
			if err != nil {
				log.Warnf("deadline %v", err)
			} else {
				data = append(data, deadlines)
			}
		}(m, tag)
	}
	wg.Wait()
	if len(data) == 0 {
		return nil, errors.New("no miner deadlines result")
	}
	return data, nil
}

type Deadlines struct {
	MinerDeadlineOpenTime         *prometheus.GaugeVec
	MinerDeadlineSectorsRecovery  *prometheus.GaugeVec
	MinerDeadlineSectorsFaulty    *prometheus.GaugeVec
	MinerDeadlineSectorsAll       *prometheus.GaugeVec
	MinerDeadlineSectorsActive    *prometheus.GaugeVec
	MinerDeadlinePartitions       *prometheus.GaugeVec
	MinerDeadlinePartitionsProven *prometheus.GaugeVec
	MinerDeadlineOpenEpoch        *prometheus.GaugeVec
	MinerDeadlinesInfo            *prometheus.GaugeVec
	native                        []*DeadlinesMetrics
}

func NewDeadlinesCollector(data []*DeadlinesMetrics) (metrics.LotusCollector, error) {

	var deadlineLabel = []string{"miner_id", "miner_tag", "index"}
	return &Deadlines{
		MinerDeadlineOpenTime: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_deadline_open_time",
				Help:      "return miner deadline open timestamp(unix).",
			}, deadlineLabel),
		MinerDeadlineSectorsRecovery: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_deadline_sector_recovery",
				Help:      "return miner deadline recovery sector number.",
			}, deadlineLabel),
		MinerDeadlineSectorsFaulty: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_deadline_sector_faulty",
				Help:      "return miner deadline faulty sector number.",
			}, deadlineLabel),
		MinerDeadlineSectorsAll: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_deadline_sector_all",
				Help:      "return miner deadline all sector number.",
			}, deadlineLabel),
		MinerDeadlineSectorsActive: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_deadline_sector_active",
				Help:      "return miner deadline active sector number.",
			}, deadlineLabel),
		MinerDeadlinePartitions: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_deadline_partitions",
				Help:      "return miner deadline partitions number.",
			}, deadlineLabel),
		MinerDeadlinePartitionsProven: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_deadline_partitions_proven",
				Help:      "return miner deadline proven partitions number.",
			}, deadlineLabel),
		MinerDeadlineOpenEpoch: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_deadline_open_epoch",
				Help:      "return miner deadline open epoch.",
			}, deadlineLabel),
		MinerDeadlinesInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_deadline_info",
				Help:      "return miner current deadline cost(-1:all partition has proven,-2:some partition was not proven,other:cost height).",
			}, []string{"miner_id", "miner_tag", "period_start", "index", "open_epoch"}),
		native: data,
	}, nil

}
func (d *Deadlines) Update(ch chan<- prometheus.Metric) {
	d.MinerDeadlineOpenTime.Reset()
	d.MinerDeadlineSectorsRecovery.Reset()
	d.MinerDeadlineSectorsFaulty.Reset()
	d.MinerDeadlineSectorsAll.Reset()
	d.MinerDeadlineSectorsActive.Reset()
	d.MinerDeadlinePartitions.Reset()
	d.MinerDeadlinePartitionsProven.Reset()
	d.MinerDeadlineOpenEpoch.Reset()
	d.MinerDeadlinesInfo.Reset()
	for _, dd := range d.native {
		d.MinerDeadlinesInfo.WithLabelValues(dd.ActorStr, dd.ActorTag, dd.PeriodStart, dd.CurrentIdx, dd.CurrentOpenEpoch).Set(dd.CurrentCost)
		for _, dl := range dd.DlsMetrics {
			d.MinerDeadlineOpenEpoch.WithLabelValues(dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.OpenEpoch)
			d.MinerDeadlineOpenTime.WithLabelValues(dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.OpenTime)
			d.MinerDeadlineSectorsRecovery.WithLabelValues(dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.Recovery)
			d.MinerDeadlineSectorsFaulty.WithLabelValues(dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.Faults)
			d.MinerDeadlineSectorsAll.WithLabelValues(dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.Sectors)
			d.MinerDeadlineSectorsActive.WithLabelValues(dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.ActiveSectors)
			d.MinerDeadlinePartitions.WithLabelValues(dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.Partitions)
			d.MinerDeadlinePartitionsProven.WithLabelValues(dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.Proven)
		}
	}
	d.MinerDeadlineOpenTime.Collect(ch)
	d.MinerDeadlineSectorsRecovery.Collect(ch)
	d.MinerDeadlineSectorsFaulty.Collect(ch)
	d.MinerDeadlineSectorsAll.Collect(ch)
	d.MinerDeadlineSectorsActive.Collect(ch)
	d.MinerDeadlinePartitions.Collect(ch)
	d.MinerDeadlinePartitionsProven.Collect(ch)
	d.MinerDeadlineOpenEpoch.Collect(ch)
	d.MinerDeadlinesInfo.Collect(ch)

}
