package fullnode

import (
	"errors"
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/prometheus/client_golang/prometheus"
	"sync"
)

const (
	AttoFilTOFil                 = float64(1000000000000000000)
	EpochDurationSeconds         = 30
	SecondsInHour                = 60 * 60
	SecondsInDay                 = 24 * SecondsInHour
	EpochsInHour                 = SecondsInHour / EpochDurationSeconds
	EpochsInDay                  = 24 * EpochsInHour
	EpochsInYear                 = 365 * EpochsInDay
	MaxSectorExpirationExtension = 540 * EpochsInDay
)

type SectorsMetrics struct {
	SectorRemainingLifeCount         *prometheus.GaugeVec // 周期内的扇区总数
	SectorRemainingLifeInitialPledge *prometheus.GaugeVec // 周期内的扇区质押数

	// EarliestSectorInfo 和 LatestSectorInfo 用于描述周期内最早和最晚的扇区信息. 值是时间戳
	EarliestSectorInfo *prometheus.GaugeVec // 最早到期扇区的信息
	LatestSectorInfo   *prometheus.GaugeVec // 最晚到期扇区的信息

	// 下面用于每个扇区的详情.
	SectorActivation    *prometheus.GaugeVec // 扇区激活的Epoch
	SectorExpiration    *prometheus.GaugeVec // 扇区终止的Epoch
	SectorInitialPledge *prometheus.GaugeVec // 扇区质押
	native              MinerSector
}

func NewSectorMetrics(data MinerSector) (*SectorsMetrics, error) {
	if len(data.Result) == 0 {
		return nil, errors.New("no miner sectors found")
	}
	return &SectorsMetrics{
		SectorRemainingLifeCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_sector_remaining_life_count",
				Help:      "number of sectors in this life.",
			}, []string{"miner_id", "miner_tag", "life"}),
		SectorRemainingLifeInitialPledge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_sector_remaining_life_initial_pledge",
				Help:      "initial pledge sum in this life.",
			}, []string{"miner_id", "miner_tag", "life"}),
		EarliestSectorInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_sector_earliest_expiration",
				Help:      "the latest sector expiration (timestamp)",
			}, []string{"miner_id", "miner_tag", "epoch", "sector_number"}),
		LatestSectorInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_sector_latest_expiration",
				Help:      "the latest sector expiration (timestamp)",
			}, []string{"miner_id", "miner_tag", "epoch", "sector_number"}),
		SectorActivation: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_sector_activation",
				Help:      "the sector activation on epoch",
			}, []string{"miner_id", "miner_tag", "sector_number"}),
		SectorExpiration: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_sector_expiration",
				Help:      "the sector expiration on epoch",
			}, []string{"miner_id", "miner_tag", "sector_number"}),
		SectorInitialPledge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_sector_initial_pledge",
				Help:      "sector initial pledge (fil).",
			}, []string{"miner_id", "miner_tag", "sector_number"}),
		native: data,
	}, nil
}

func (m *SectorsMetrics) Update(ch chan<- prometheus.Metric) {
	m.SectorRemainingLifeCount.Reset()
	m.SectorRemainingLifeInitialPledge.Reset()
	m.EarliestSectorInfo.Reset()
	m.LatestSectorInfo.Reset()
	m.SectorActivation.Reset()
	m.SectorExpiration.Reset()
	m.SectorInitialPledge.Reset()

	for _, i := range m.native.Result {
		minerId := i.Miner.ActorID.String()
		tag := i.Miner.Tag

		for sLife, n := range i.CountSummary {
			m.SectorRemainingLifeCount.WithLabelValues(minerId, tag, sLife).Set(float64(n))
		}
		for sLife, n := range i.InitialPledgeSummary {
			m.SectorRemainingLifeInitialPledge.WithLabelValues(minerId, tag, sLife).Set(n)
		}
		m.EarliestSectorInfo.WithLabelValues(
			minerId, tag, i.EarliestSector.Expiration.String(), i.EarliestSector.SectorNumber.String()).Set(
			float64(uint64(i.EarliestSector.Expiration-m.native.BaseEpoch)*30 + m.native.BaseTimestamp))
		m.LatestSectorInfo.WithLabelValues(
			minerId, tag, i.LatestSector.Expiration.String(), i.LatestSector.SectorNumber.String()).Set(
			float64(uint64(i.LatestSector.Expiration-m.native.BaseEpoch)*30 + m.native.BaseTimestamp))
		for _, s := range i.Detail {
			m.SectorActivation.WithLabelValues(minerId, tag, s.SectorNumber.String()).Set(float64(s.Activation))
			m.SectorExpiration.WithLabelValues(minerId, tag, s.SectorNumber.String()).Set(float64(s.Expiration))
			m.SectorInitialPledge.WithLabelValues(minerId, tag, s.SectorNumber.String()).Set(float64(s.InitialPledge.Int64()))
		}
	}

	m.SectorRemainingLifeCount.Collect(ch)
	m.SectorRemainingLifeInitialPledge.Collect(ch)
	m.EarliestSectorInfo.Collect(ch)
	m.LatestSectorInfo.Collect(ch)
	m.SectorActivation.Collect(ch)
	m.SectorExpiration.Collect(ch)
	m.SectorInitialPledge.Collect(ch)
}

func CreateBuckets(num int) []int {

	var res []int
	quantile := MaxSectorExpirationExtension / num

	// 把1d和7d的加上
	res = append(res, EpochsInDay)
	res = append(res, EpochsInDay*7)

	for i := 1; i <= num; i++ {
		res = append(res, quantile*i)
	}
	if res[len(res)-1] < MaxSectorExpirationExtension {
		res = append(res, MaxSectorExpirationExtension)
	}
	return res
}

type MinerSector struct {
	//DetailNum     int
	BaseTimestamp uint64
	BaseEpoch     abi.ChainEpoch
	Result        []SectorOnChain
}

type SectorOnChain struct {
	Miner                ActorBase
	CountSummary         map[string]int
	InitialPledgeSummary map[string]float64
	EarliestSector       *miner.SectorOnChainInfo
	LatestSector         *miner.SectorOnChainInfo
	Detail               []*miner.SectorOnChainInfo
}

func (l *FullNode) sectorInfo() (MinerSector, error) {
	buckets := CreateBuckets(l.conf.SectorExpirationBuckets)

	var miners = make(map[address.Address]ActorBase)
	var currEpoch = l.chain.chainHead.Height()
	for addr, m := range l.chain.attentionActor {
		if m.ActorType == MinerKey {
			miners[addr] = m
		}
	}

	var allMinerSectorsInfo []SectorOnChain
	wg := sync.WaitGroup{}
	wg.Add(len(miners))
	for addr, m := range miners {

		go func(addr address.Address, m ActorBase) {
			defer wg.Done()
			var count = make(map[int]int)
			var initialPledge = make(map[int]float64)
			var latestIndex, earliestIndex int
			sectors, err := l.api.StateMinerActiveSectors(l.ctx, addr, l.chain.chainHead.Key())
			if err != nil {
				log.Warnf("%v api StateMinerActiveSectors  err:%v", m.ActorID, err)
				return
			}

			if len(sectors) == 0 {
				return
			}

			for idx, sector := range sectors {
				// 跳过已到期的扇区.
				if sector.Expiration < currEpoch {
					continue
				}

				if sector.Expiration <= sectors[earliestIndex].Expiration {
					earliestIndex = idx
				}

				if sector.Expiration >= sectors[latestIndex].Expiration {
					latestIndex = idx
				}

				sectorInitialPledge := float64(sector.InitialPledge.Int64()) / AttoFilTOFil
				// 对扇区判断有效期, 并进行汇总处理.
				for _, e := range buckets {
					if int64(sector.Expiration-currEpoch) <= int64(e) {
						count[e]++
						initialPledge[e] += sectorInitialPledge
						break
					}
				}
			}

			var countString = make(map[string]int)
			var initialPledgeString = make(map[string]float64)
			for b, c := range count {
				str := fmt.Sprintf("le_%03dd", b/EpochsInDay)
				countString[str] = c
			}

			for b, c := range initialPledge {
				str := fmt.Sprintf("le_%03dd", b/EpochsInDay)
				initialPledgeString[str] = c
			}
			var details = sectors
			if l.conf.SectorDetailNum < len(sectors) {
				details = sectors[:l.conf.SectorDetailNum]
			}

			allMinerSectorsInfo = append(allMinerSectorsInfo, SectorOnChain{
				Miner:                m,
				CountSummary:         countString,
				InitialPledgeSummary: initialPledgeString,
				EarliestSector:       sectors[earliestIndex],
				LatestSector:         sectors[latestIndex],
				Detail:               details,
			})

		}(addr, m)

	}

	wg.Wait()

	return MinerSector{
		BaseTimestamp: l.chain.chainHead.Blocks()[0].Timestamp,
		BaseEpoch:     l.chain.chainHead.Blocks()[0].Height,
		Result:        allMinerSectorsInfo,
	}, nil
}
