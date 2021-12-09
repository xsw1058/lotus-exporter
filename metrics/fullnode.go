package metrics

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	logging "github.com/ipfs/go-log/v2"
	"reflect"
	"strconv"
	"sync"
	"time"
)

var (
	logger = logging.Logger("metrics")
)

type fullNodeApi struct {
	api    *api.FullNodeStruct
	tipSet types.TipSetKey
	ctx    context.Context
}

type dlMetrics struct {
	Index      string  // deadline ID
	Partitions float64 // 分区数
	Sectors    float64 // 总扇区数
	Faults     float64 // 错误扇区数
	Recovery   float64 // 恢复扇区数
	Proven     float64 // proven partition
	OpenTime   float64 // 开始时间
	OpenEpoch  float64 // 开始高度
}
type ActorDeadlines struct {
	DurationSeconds  float64
	ActorStr         string // 矿工号
	ActorTag         string
	CurrentIdx       string      // 当前deadline ID
	PeriodStart      string      // 当前周期开始的epoch
	CurrentOpenEpoch string      // 当前窗口的挑战字epoch
	CurrentCost      float64     // 只计算当前窗口的耗时
	DlsMetrics       []dlMetrics // 每个窗口的信息
}
type ActorInfo struct {
	DurationSeconds float64
	ActorStr        string // 矿工号
	ActorTag        string
	PeerId          string
	SectorSize      float64 // 扇区大小
	HasMinPower     int
	RawBytePower    float64
	QualityAdjPower float64
	Bls             []BalanceMetrics
}
type BalanceMetrics struct {
	Tag       string
	Address   string
	AccountId string
	Balance   float64
}

type FullNodeInfo struct {
	DurationSeconds float64
	Version         string
	NetworkVersion  float64
	SyncState       []syncMetrics
}

type syncMetrics struct {
	WorkerID   string
	Status     string
	HeightDiff float64
}

func NewFullNodeApi(api *api.FullNodeStruct, apiCtx context.Context, tipSet types.TipSetKey) *fullNodeApi {
	return &fullNodeApi{
		api:    api,
		tipSet: tipSet,
		ctx:    apiCtx,
	}
}

func (a *fullNodeApi) GetFullNodeInfo(ch chan FullNodeInfo) {
	defer close(ch)
	start := time.Now()
	version, err := a.api.Version(a.ctx)
	if err != nil {
		logger.Warnf("get full node version:%v", err)
		return
	}
	networkVersion, err := a.api.StateNetworkVersion(a.ctx, a.tipSet)
	if err != nil {
		logger.Warnf("get full node network version:%v", err)
		return
	}

	syncState, err := a.api.SyncState(a.ctx)
	if err != nil {
		logger.Warnf("get sync state failed: %v", err)
		return
	}
	c := FullNodeInfo{
		DurationSeconds: float64(0),
		Version:         version.String(),
		NetworkVersion:  float64(networkVersion),
		SyncState:       nil,
	}
	for idx, ss := range syncState.ActiveSyncs {
		var heightDiff int64
		if ss.Base != nil {
			heightDiff = int64(ss.Base.Height())
		}
		if ss.Target != nil {
			heightDiff = int64(ss.Target.Height()) - heightDiff
		} else {
			heightDiff = 0
		}
		c.SyncState = append(c.SyncState, syncMetrics{
			WorkerID:   strconv.Itoa(idx),
			Status:     ss.Stage.String(),
			HeightDiff: float64(heightDiff),
		})
	}
	c.DurationSeconds = time.Now().Sub(start).Seconds()
	ch <- c
}

func (a *fullNodeApi) actorDeadlinesMetrics(actor address.Address, tag string, baseEpoch float64, baseTimestamp float64, ch chan ActorDeadlines, wg *sync.WaitGroup) {
	defer wg.Done()
	start := time.Now()
	actorStr := actor.String()
	di, err := a.api.StateMinerProvingDeadline(a.ctx, actor, a.tipSet)
	if err != nil {
		logger.Warnf("get %s di :%v", actorStr, err)
		return
	}
	dls, err := a.api.StateMinerDeadlines(a.ctx, actor, a.tipSet)
	if err != nil {
		logger.Warnf("get %s actorDls :%v", actorStr, err)
		return
	}

	d := ActorDeadlines{
		DurationSeconds:  float64(0),
		ActorStr:         actorStr,
		ActorTag:         tag,
		CurrentIdx:       strconv.Itoa(int(di.Index)),
		PeriodStart:      di.PeriodStart.String(),
		CurrentOpenEpoch: di.Open.String(),
		CurrentCost:      float64(0),
		DlsMetrics:       nil,
	}

	for dlIdx, dl := range dls {
		partitions, err := a.api.StateMinerPartitions(a.ctx, actor, uint64(dlIdx), a.tipSet)
		if err != nil {
			logger.Warnf("getting %s Partitions for deadline %d: %s", actorStr, dlIdx, err)
			return
		}

		// 忽略没有分区的窗口。
		if len(partitions) < 1 {
			if di.Index == uint64(dlIdx) {
				d.CurrentCost = -1
			}
			continue
		}
		openEpoch := float64(di.PeriodStart) + (float64(dlIdx) * 60)
		openTime := (openEpoch-baseEpoch)*30 + baseTimestamp

		provenPartitions, err := dl.PostSubmissions.Count()
		if err != nil {
			logger.Warnf("getting %s provenPartitions: %s", actorStr, err)
			return
		}
		sectors := float64(0)
		faults := float64(0)
		recovery := float64(0)

		// 只计算当前窗口的耗时。
		if di.Index == uint64(dlIdx) {
			// 未提交证明时，窗口数量大于已证明的窗口数量
			if uint64(len(partitions)) > provenPartitions {
				// 该值是当前高度-本轮窗口的开始高度。这个值会随着时间的递增而递增。
				d.CurrentCost = float64(di.CurrentEpoch) - openEpoch
				logger.Debugw("wait proven", "miner", actorStr,"index",dlIdx,"cost",d.CurrentCost)
			}
			// 提交证明后，窗口数量等于已证明的窗口数量
			if uint64(len(partitions)) == provenPartitions {
				d.CurrentCost = -1
			}
		}
		for _, partition := range partitions {
			sc, err := partition.AllSectors.Count()
			if err != nil {
				logger.Warnf("getting %s AllSectors: %s", actorStr, err)
				continue
			}
			sectors += float64(sc)

			fc, err := partition.FaultySectors.Count()
			if err != nil {
				logger.Warnf("getting %s FaultySectors: %s", actorStr, err)
				continue
			}
			faults += float64(fc)

			rec, err := partition.RecoveringSectors.Count()
			if err != nil {
				logger.Warnf("getting %s RecoveringSectors: %s", actorStr, err)
				continue
			}
			recovery += float64(rec)
		}
		d.DlsMetrics = append(d.DlsMetrics, dlMetrics{
			Index:      strconv.Itoa(dlIdx),
			Partitions: float64(len(partitions)),
			Sectors:    sectors,
			Faults:     faults,
			Recovery:   recovery,
			Proven:     float64(provenPartitions),
			OpenTime:   openTime,
			OpenEpoch:  openEpoch,
		})
	}
	d.DurationSeconds = time.Now().Sub(start).Seconds()
	ch <- d
}

func (a *fullNodeApi) GetActorsDeadlinesMetrics(actors map[address.Address]string, baseEpoch float64, baseTimestamp float64, ch chan ActorDeadlines) {
	defer close(ch)
	wg := sync.WaitGroup{}
	for actor, tag := range actors {
		wg.Add(1)
		go a.actorDeadlinesMetrics(actor, tag, baseEpoch, baseTimestamp, ch, &wg)
	}
	wg.Wait()
}

func (a *fullNodeApi) GetActorsInfoMetrics(actors map[address.Address]string, ch chan ActorInfo) {
	defer close(ch)
	wg := sync.WaitGroup{}
	for actor, tag := range actors {
		wg.Add(1)
		go a.actorInfoMetrics(actor, tag, ch, &wg)
	}
	wg.Wait()
}

func (a *fullNodeApi) GetExternalWallet(wallets map[address.Address]string, ch chan BalanceMetrics) {
	defer close(ch)
	wg := sync.WaitGroup{}
	for wallet, tag := range wallets {
		wg.Add(1)
		go func(wallet address.Address, tag string) {
			defer wg.Done()
			walletStr := wallet.String()
			walletBalance, err := a.api.WalletBalance(a.ctx, wallet)
			if err != nil {
				logger.Warnf("get wallet(%s) balance :%v", walletStr, err)
				return
			}
			id, err := a.api.StateLookupID(a.ctx, wallet, a.tipSet)
			if err != nil {
				logger.Warnf("get wallet(%s) ID :%v", walletStr, err)
			}

			b, err := strconv.ParseFloat(types.FIL(walletBalance).Unitless(), 64)
			if err != nil {
				logger.Warnf("failed decode wallet(%s) to fil: %v", walletStr, err)
			}
			ch <- BalanceMetrics{
				Tag:       tag,
				Address:   walletStr,
				AccountId: id.String(),
				Balance:   b,
			}
		}(wallet, tag)
	}
	wg.Wait()
}
func (a *fullNodeApi) actorInfoMetrics(actor address.Address, tag string, ch chan ActorInfo, wg *sync.WaitGroup) {
	defer wg.Done()
	start := time.Now()
	actorStr := actor.String()
	mi, err := a.api.StateMinerInfo(a.ctx, actor, a.tipSet)
	if err != nil {
		logger.Warnf("get miner(%s) fullNodeInfo failed:%v", actorStr, err)
		return
	}
	m := ActorInfo{
		DurationSeconds: float64(0),
		ActorStr:        actorStr,
		ActorTag:        tag,
		PeerId:          mi.PeerId.String(),
		SectorSize:      float64(mi.SectorSize),
		Bls:             nil,
	}

	availableBalance, err := a.api.StateMinerAvailableBalance(a.ctx, actor, a.tipSet)
	if err != nil {
		logger.Warnf("get %s availabel Balance:%v", actor.String(), err)
		return
	}

	bl, err := strconv.ParseFloat(types.FIL(availableBalance).Unitless(), 64)
	if err != nil {
		logger.Warnf("failed decode wallet(%s) to fil: %v", actor.String(), err)
	}

	m.Bls = append(m.Bls, BalanceMetrics{
		Tag:       "available",
		Address:   actorStr,
		AccountId: actorStr,
		Balance:   bl,
	})

	power, err := a.api.StateMinerPower(a.ctx, actor, a.tipSet)
	if err != nil {
		logger.Warnf("get actor(%s) miner power failed: %v", actor.String(), err)
		return
	}
	var h = 0
	if power.HasMinPower {
		h = 1
	}
	m.QualityAdjPower = float64(power.MinerPower.QualityAdjPower.Int64())
	m.RawBytePower = float64(power.MinerPower.RawBytePower.Int64())
	m.HasMinPower = h

	// 构造miner所有owner、worker、control地址的map
	addrIDs := make(map[string]address.Address, 4)
	if reflect.DeepEqual(mi.Worker, mi.Owner) {
		addrIDs["owner"] = mi.Owner
	} else {
		addrIDs["owner"] = mi.Owner
		addrIDs["worker"] = mi.Worker
	}

	if len(mi.ControlAddresses) > 0 {
		for i, a := range mi.ControlAddresses {
			if reflect.DeepEqual(a, mi.Owner) || reflect.DeepEqual(a, mi.Worker) {
				continue
			}
			addrIDs["control"+strconv.Itoa(i)] = a
		}
	}
	for walletTag, id := range addrIDs {
		addr, err := a.api.StateAccountKey(a.ctx, id, a.tipSet)
		if err != nil {
			logger.Warnf("get account(%s) key failed:%v", id, err)
			return
		}

		walletBalance, err := a.api.WalletBalance(a.ctx, addr)
		if err != nil {
			logger.Warnf("failed get wallet(%s) Balance: %v", addr.String(), err)
		}

		// 从lotus 的bigInt解析为float64，单位为Fil
		b, err := strconv.ParseFloat(types.FIL(walletBalance).Unitless(), 64)
		if err != nil {
			logger.Warnf("failed decode wallet(%s) to fil: %v", addr.String(), err)
		}
		m.Bls = append(m.Bls, BalanceMetrics{
			Tag:       walletTag,
			Address:   addr.String(),
			AccountId: id.String(),
			Balance:   b,
		})
	}
	m.DurationSeconds = time.Now().Sub(start).Seconds()
	//a.wg.Add(1)
	ch <- m
}
