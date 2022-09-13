package metrics

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"github.com/xsw1058/lotus-exporter/message"
	"reflect"
	"strconv"
	"sync"
	"time"
)

var (
	logger = logging.Logger("metrics")
)

type fullNodeApi struct {
	api          *api.FullNodeStruct
	tipSet       types.TipSetKey
	ctx          context.Context
	ciDs         map[string]cid.Cid
	localWallets []address.Address
}

type dlMetrics struct {
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
	ActorTag        string // 别名
	PeerId          string
	SectorSize      float64 // 扇区大小
	HasMinPower     int
	RawBytePower    float64          // 原值算力
	QualityAdjPower float64          // 有效算力
	Bls             []BalanceMetrics // 各种余额
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
		api:          api,
		tipSet:       tipSet,
		ctx:          apiCtx,
		ciDs:         nil,
		localWallets: nil,
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

		//isEmpty, err := dl.PostSubmissions.IsEmpty()
		//if err != nil {
		//	logger.Warnf("getting %s provenPartitions: %s", actorStr, err)
		//	continue
		//}

		provenPartitions, err := dl.PostSubmissions.Count()
		if err != nil {
			logger.Warnf("getting %s provenPartitions: %s", actorStr, err)
			return
		}
		sectors := float64(0)
		actives := float64(0)
		faults := float64(0)
		recovery := float64(0)

		haveActiveSectorPartitions := uint64(0)

		for _, partition := range partitions {
			sc, err := partition.AllSectors.Count()
			if err != nil {
				logger.Warnf("getting %s AllSectors: %s", actorStr, err)
				continue
			}
			sectors += float64(sc)

			active, err := partition.ActiveSectors.Count()
			if err != nil {
				logger.Warnf("getting %s ActiveSectors: %s", actorStr, err)
				continue
			}
			actives += float64(active)
			if actives > 0 {
				haveActiveSectorPartitions += 1
			}
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

		// 只计算当前窗口的耗时。
		if di.Index == uint64(dlIdx) {

			// 有活动扇区的分区数大于提交证明的分区数 表示还没有做完证明
			if haveActiveSectorPartitions > provenPartitions {
				d.CurrentCost = float64(di.CurrentEpoch) - openEpoch
				logger.Debugw("proven cost", "miner", actorStr, "index", dlIdx, "cost", d.CurrentCost)
			} else {
				// 总分区数 == 提交证明的分区数
				if uint64(len(partitions)) == provenPartitions {
					d.CurrentCost = -1
					// 总分区数 > 提交证明的分区数
				} else if uint64(len(partitions)) > provenPartitions {
					d.CurrentCost = -2
					// 其他情况
				} else {
					logger.Warnw("proven cost", "miner", actorStr, "index", dlIdx,
						"partitions", partitions,
						"provenPartitions", provenPartitions,
						"haveActiveSectorPartitions", haveActiveSectorPartitions)
					d.CurrentCost = -3
				}
			}
		}

		d.DlsMetrics = append(d.DlsMetrics, dlMetrics{
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
		return
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
	// 获取owner、worker、control钱包余额。
	for walletTag, id := range addrIDs {
		act, err := a.api.StateGetActor(a.ctx, id, a.tipSet)
		if err != nil {
			logger.Warnf("get accout(%v) actor failed:%v", id, err)
			continue
		}

		// 从lotus 的bigInt解析为float64，单位为Fil
		balance, err := strconv.ParseFloat(types.FIL(act.Balance).Unitless(), 64)
		if err != nil {
			logger.Warnf("failed decode wallet(%s) to fil: %v", id, err)
			continue
		}

		var addrStr = "unknown"
		if name, _, ok := actors.GetActorMetaByCode(act.Code); ok {
			switch name {
			case actors.MultisigKey:
				addrStr = id.String()
			case actors.AccountKey:
				addr, err := a.api.StateAccountKey(a.ctx, id, a.tipSet)
				if err != nil {
					logger.Warnf("get account(%s) key failed:%v", id, err)
					continue
				}
				addrStr = addr.String()
			}
		}

		m.Bls = append(m.Bls, BalanceMetrics{
			Tag:       walletTag,
			Address:   addrStr,
			AccountId: id.String(),
			Balance:   balance,
		})
	}

	// 获取抵押、锁仓等余额
	{
		mact, err := a.api.StateGetActor(a.ctx, actor, types.EmptyTSK)
		if err != nil {
			logger.Warnw("metric", "get actor", err)
			return
		}

		tbs := blockstore.NewTieredBstore(blockstore.NewAPIBlockstore(a.api), blockstore.NewMemory())
		mas, err := miner.Load(adt.WrapStore(a.ctx, cbor.NewCborStore(tbs)), mact)
		if err != nil {
			logger.Warnw("metric", "miner load", err)
			return
		}

		lockedFunds, err := mas.LockedFunds()
		if err != nil {
			logger.Warnw("metric", "mas get locked funds", err)
			return
		}

		// 预提交的抵押
		preCommit, err := strconv.ParseFloat(types.FIL(lockedFunds.PreCommitDeposits).Unitless(), 64)
		if err != nil {
			logger.Warnw("metric", "parse preCommit", err)
			return
		}
		m.Bls = append(m.Bls, BalanceMetrics{
			Tag:       "pre_commit",
			Address:   actorStr,
			AccountId: actorStr,
			Balance:   preCommit,
		})

		// 抵押余额
		pledge, err := strconv.ParseFloat(types.FIL(lockedFunds.InitialPledgeRequirement).Unitless(), 64)
		if err != nil {
			logger.Warnw("metric", "parse pledge", err)
			return
		}
		m.Bls = append(m.Bls, BalanceMetrics{
			Tag:       "pledge",
			Address:   actorStr,
			AccountId: actorStr,
			Balance:   pledge,
		})

		// 锁仓余额
		vesting, err := strconv.ParseFloat(types.FIL(lockedFunds.VestingFunds).Unitless(), 64)
		if err != nil {
			logger.Warnw("metric", "parse vesting", err)
			return
		}
		m.Bls = append(m.Bls, BalanceMetrics{
			Tag:       "vesting",
			Address:   actorStr,
			AccountId: actorStr,
			Balance:   vesting,
		})

		// 总余额
		balanceAll, err := strconv.ParseFloat(types.FIL(mact.Balance).Unitless(), 64)
		if err != nil {
			logger.Warnw("metric", "parse vesting", err)
			return
		}
		m.Bls = append(m.Bls, BalanceMetrics{
			Tag:       "balance",
			Address:   actorStr,
			AccountId: actorStr,
			Balance:   balanceAll,
		})
	}
	m.DurationSeconds = time.Now().Sub(start).Seconds()
	ch <- m
}

type localMessage struct {
	*types.SignedMessage
	MethodStr string
}

type MessageMetric struct {
	DurationSeconds float64
	MessageTotal    int
	LocalMessages   []localMessage
}

func (a *fullNodeApi) MessagePool(ch chan MessageMetric) {
	defer close(ch)
	startTime := time.Now()
	if a.ciDs == nil || a.localWallets == nil {

		networkVersion, err := a.api.StateNetworkVersion(a.ctx, a.tipSet)
		if err != nil {
			logger.Warnf("api StateNetworkVersion:%v", err)
			return
		}

		ciDsGet, err := a.api.StateActorCodeCIDs(a.ctx, networkVersion)
		if err != nil {
			logger.Warnf("api StateActorCodeCIDs:%v", err)
			return
		}

		a.ciDs = ciDsGet

		walletList, err := a.api.WalletList(a.ctx)
		if err != nil {
			logger.Warnf("api WalletList:%v", err)
			return
		}
		a.localWallets = walletList
	}

	messages, err := a.api.MpoolPending(a.ctx, a.tipSet)
	if err != nil {
		logger.Warnf("api MpoolPending:%v", err)
		return
	}
	var localMessages []localMessage
	for _, m := range messages {
		for _, w := range a.localWallets {
			if m.Message.From == w {
				actor, err := a.api.StateGetActor(a.ctx, m.Message.To, a.tipSet)
				if err != nil {
					logger.Warnf("api StateGetActor:%v", err)
					continue
				}

				methodStr, err := message.GetMessageMethodStringByNum(a.ciDs, actor.Code.String(), m.Message.Method)
				if err != nil {
					logger.Warnf("GetMessageMethodStringByNum:%v", err)
					continue
				}
				localMessages = append(localMessages, localMessage{
					SignedMessage: m,
					MethodStr:     methodStr,
				})
			}
		}
	}
	c := MessageMetric{
		DurationSeconds: time.Now().Sub(startTime).Seconds(),
		MessageTotal:    len(messages),
		LocalMessages:   localMessages,
	}
	ch <- c
}
