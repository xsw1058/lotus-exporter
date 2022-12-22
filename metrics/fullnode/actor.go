package fullnode

import (
	"errors"
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xsw1058/lotus-exporter/metrics"
	"strconv"
	"sync"
)

type ActorMetrics struct {
	Actor ActorBase
	// 如果是矿工,则显示矿工的相关信息
	Miner   MinerMetric
	Balance float64
}

type ActorBase struct {
	Tag string
	// StateLookupID 可以获取一个地址的ID
	ActorID address.Address
	// 先通过 StateGetActor, 再和StateActorCodeCIDs 进行比对.
	ActorType string
	// StateAccountKey 的公钥. 仅account类型的具有
	// 如果是miner或multisig 则表示RobustAddress地址
	key address.Address
	// 用于表示关联的Miner
	MinerID address.Address
}

type MinerMetric struct {
	SectorSize      float64
	HasMinPower     bool
	RawBytePower    float64
	QualityAdjPower float64
	// 可用余额
	AvailableBalance float64
	// 锁仓奖励
	VestingFunds float64
	// 扇区质押
	InitialPledgeRequirement float64
	// 预存款
	PreCommitDeposits float64
}

func (l *FullNode) getActors(addr address.Address, tag string) ([]ActorBase, error) {

	addrID, err := l.api.StateLookupID(l.ctx, addr, l.chain.chainHead.Key())
	if err != nil {
		return nil, err
	}

	actor, err := l.api.StateGetActor(l.ctx, addrID, l.chain.chainHead.Key())
	if err != nil {
		return nil, err
	}

	var abs []ActorBase
	//var ab = ActorBase{
	//	Tag:     tag,
	//	ActorID: addrID,
	//}
	var ab = ActorBase{
		Tag:       tag,
		ActorID:   addrID,
		ActorType: "",
		key:       address.Address{},
		MinerID:   addrID,
	}

	for k, c := range l.chain.ciDs {
		if actor.Code == c {
			ab.ActorType = k
		}
	}

	switch ab.ActorType {
	case actors.MinerKey:
		robustAddress, err := l.api.StateLookupRobustAddress(l.ctx, addrID, l.chain.chainHead.Key())
		if err != nil {
			return nil, err
		}
		ab.key = robustAddress

		mi, err := l.api.StateMinerInfo(l.ctx, addrID, l.chain.chainHead.Key())
		if err != nil {
			return nil, err
		}
		var as = make(map[address.Address]string)

		for i, a := range mi.ControlAddresses {
			as[a] = tag + "_control" + strconv.Itoa(i)
		}
		as[mi.Worker] = tag + "_worker"
		as[mi.Beneficiary] = tag + "_beneficiary"
		as[mi.Owner] = tag + "_owner"

		for a, t := range as {
			actorBases, err := l.getActors(a, t)
			if err != nil {
				log.Warnln(err)
				continue
			}

			for _, subActor := range actorBases {
				subActor.MinerID = addrID
				abs = append(abs, subActor)

			}

		}

	case actors.AccountKey:
		accountKey, err := l.api.StateAccountKey(l.ctx, addrID, l.chain.chainHead.Key())
		if err != nil {
			return nil, err
		}
		ab.key = accountKey
	case actors.MultisigKey:
		robustAddress, err := l.api.StateLookupRobustAddress(l.ctx, addrID, l.chain.chainHead.Key())
		if err != nil {
			return nil, err
		}
		ab.key = robustAddress
	default:
		return nil, errors.New(fmt.Sprintf("%v type %v is unsupport", addr, ab.ActorType))
	}

	abs = append(abs, ab)
	return abs, nil
}

func (l *FullNode) appendAttentionActor(actor ActorBase) {
	l.chain.Lock()
	defer l.chain.Unlock()

	if v, ok := l.chain.attentionActor[actor.ActorID]; ok {
		switch v.Tag {
		case actor.Tag:
			return
		default:
			newTag := v.Tag + "+" + actor.Tag
			actor.Tag = newTag
		}
	}

	l.chain.attentionActor[actor.ActorID] = actor
}

func (l *FullNode) isAttentionActor(addr address.Address) (*ActorBase, bool) {
	attentionActor := l.getAttentionActor()
	for _, attention := range attentionActor {
		if attention.ActorID == addr || attention.key == addr {
			return &attention, true
		}
	}
	return nil, false
}

func (l *FullNode) checkAllInAttentionActors(addrs []address.Address) bool {
	for _, addr := range addrs {
		_, ok := l.isAttentionActor(addr)
		if ok != true {
			return false
		}
	}
	return true
}

func (l *FullNode) RegisterAttentionActor(actors []ActorBase) {
	for _, ac := range actors {
		l.appendAttentionActor(ac)
	}
	log.Debugw("register attention actor done", "store len", len(l.chain.attentionActor), "register len", len(actors))
}
func (l *FullNode) getAllAttentionActor() []ActorBase {
	var abs []ActorBase
	for addr, tag := range l.conf.Miners {
		log.Debugw("get actor", "address", addr.String(), "tag", tag)
		actorBases, err := l.getActors(addr, tag)
		if err != nil {
			log.Errorln(err)
			continue
		}
		abs = append(abs, actorBases...)
	}
	return abs
}

func (l *FullNode) GetActorMetric() ([]ActorMetrics, error) {
	var res []ActorMetrics

	attentionActor := l.getAttentionActor()
	wg := sync.WaitGroup{}
	wg.Add(len(attentionActor))
	for id, attention := range attentionActor {

		go func(id address.Address, attention ActorBase) {
			defer wg.Done()
			switch attention.ActorType {
			case actors.MinerKey:
				a, err := l.getMinerMetrics(attention)
				if err != nil {
					log.Warnln(err)
					return
				}
				res = append(res, *a)
			case actors.AccountKey, actors.MultisigKey:
				act, err := l.api.StateGetActor(l.ctx, id, l.chain.chainHead.Key())
				if err != nil {
					log.Warnln(err)
					return
				}
				balance, err := strconv.ParseFloat(types.FIL(act.Balance).Unitless(), 64)
				if err != nil {
					log.Warnf("failed decode wallet(%s) to fil: %v", id, err)
					return
				}
				var s = ActorMetrics{
					Actor:   attention,
					Balance: balance,
				}
				res = append(res, s)
			default:

			}

		}(id, attention)

	}
	wg.Wait()
	return res, nil
}

func (l *FullNode) getMinerMetrics(actor ActorBase) (*ActorMetrics, error) {
	addr := actor.ActorID
	power, err := l.api.StateMinerPower(l.ctx, addr, l.chain.chainHead.Key())
	if err != nil {
		return nil, err
	}

	availableBalance, err := l.api.StateMinerAvailableBalance(l.ctx, addr, l.chain.chainHead.Key())
	if err != nil {
		return nil, err
	}
	available, err := strconv.ParseFloat(types.FIL(availableBalance).Unitless(), 64)
	if err != nil {
		return nil, err
	}

	mact, err := l.api.StateGetActor(l.ctx, addr, l.chain.chainHead.Key())
	if err != nil {
		return nil, err
	}
	balance, err := strconv.ParseFloat(types.FIL(mact.Balance).Unitless(), 64)
	if err != nil {
		return nil, err
	}
	mi, err := l.api.StateMinerInfo(l.ctx, addr, l.chain.chainHead.Key())

	var holdAddrs []address.Address
	holdAddrs = append(holdAddrs, mi.Owner)
	holdAddrs = append(holdAddrs, mi.Worker)
	holdAddrs = append(holdAddrs, mi.Beneficiary)
	holdAddrs = append(holdAddrs, mi.ControlAddresses...)
	if yes := l.checkAllInAttentionActors(holdAddrs); !yes {
		log.Infof("miner %v associated address was changed.attention actor also.", actor.ActorID)
		go func() {
			as := l.getAllAttentionActor()
			l.RegisterAttentionActor(as)
		}()

	}

	var preCommit, vesting, pledge float64
	tbs := blockstore.NewTieredBstore(blockstore.NewAPIBlockstore(l.api), blockstore.NewMemory())
	mas, err := miner.Load(adt.WrapStore(l.ctx, cbor.NewCborStore(tbs)), mact)
	if err != nil {
		return nil, err
	}
	lockedFunds, err := mas.LockedFunds()
	if err != nil {
		return nil, err
	}

	// 预提交的抵押
	preCommit, err = strconv.ParseFloat(types.FIL(lockedFunds.PreCommitDeposits).Unitless(), 64)
	if err != nil {
		return nil, err
	}

	// 抵押余额
	pledge, err = strconv.ParseFloat(types.FIL(lockedFunds.InitialPledgeRequirement).Unitless(), 64)
	if err != nil {
		return nil, err
	}

	// 锁仓余额
	vesting, err = strconv.ParseFloat(types.FIL(lockedFunds.VestingFunds).Unitless(), 64)
	if err != nil {
		return nil, err
	}

	return &ActorMetrics{
		Actor: actor,
		Miner: MinerMetric{
			SectorSize:               float64(mi.SectorSize),
			HasMinPower:              power.HasMinPower,
			RawBytePower:             float64(power.MinerPower.RawBytePower.Int64()),
			QualityAdjPower:          float64(power.MinerPower.QualityAdjPower.Int64()),
			AvailableBalance:         available,
			VestingFunds:             vesting,
			InitialPledgeRequirement: pledge,
			PreCommitDeposits:        preCommit,
		},
		Balance: balance,
	}, nil

}

type Actors struct {
	MinerRawBytePower    *prometheus.GaugeVec
	MinerQualityAdjPower *prometheus.GaugeVec
	MinerSectorSize      *prometheus.GaugeVec
	ActorBalance         *prometheus.GaugeVec
	LockedBalance        *prometheus.GaugeVec
	native               []ActorMetrics
}

func (s *Actors) Update(ch chan<- prometheus.Metric) {
	s.MinerRawBytePower.Reset()
	s.MinerQualityAdjPower.Reset()
	s.MinerSectorSize.Reset()
	s.ActorBalance.Reset()
	s.LockedBalance.Reset()

	for _, a := range s.native {
		s.ActorBalance.WithLabelValues(a.Actor.ActorID.String(), a.Actor.key.String(), a.Actor.Tag, a.Actor.ActorType, a.Actor.MinerID.String()).Set(a.Balance)
		if a.Actor.ActorType == actors.MinerKey {
			var hasPowerTag = "yes"
			if !a.Miner.HasMinPower {
				hasPowerTag = "no"
			}
			s.MinerSectorSize.WithLabelValues(a.Actor.ActorID.String(), a.Actor.Tag, hasPowerTag).Set(a.Miner.SectorSize)
			s.MinerRawBytePower.WithLabelValues(a.Actor.ActorID.String(), a.Actor.Tag).Set(a.Miner.RawBytePower)
			s.MinerQualityAdjPower.WithLabelValues(a.Actor.ActorID.String(), a.Actor.Tag).Set(a.Miner.QualityAdjPower)

			s.LockedBalance.WithLabelValues(a.Actor.ActorID.String(), a.Actor.Tag, "pledge", a.Actor.MinerID.String()).Set(a.Miner.InitialPledgeRequirement)
			s.LockedBalance.WithLabelValues(a.Actor.ActorID.String(), a.Actor.Tag, "pre_commit", a.Actor.MinerID.String()).Set(a.Miner.PreCommitDeposits)
			s.LockedBalance.WithLabelValues(a.Actor.ActorID.String(), a.Actor.Tag, "vesting", a.Actor.MinerID.String()).Set(a.Miner.VestingFunds)
			s.LockedBalance.WithLabelValues(a.Actor.ActorID.String(), a.Actor.Tag, "available", a.Actor.MinerID.String()).Set(a.Miner.AvailableBalance)
		}
	}
	s.MinerRawBytePower.Collect(ch)
	s.MinerQualityAdjPower.Collect(ch)
	s.MinerSectorSize.Collect(ch)
	s.ActorBalance.Collect(ch)
	s.LockedBalance.Collect(ch)
}

func NewActorInfo(data []ActorMetrics) (metrics.LotusCollector, error) {

	if len(data) == 0 {
		return nil, errors.New("no actor found")
	}

	return &Actors{
		MinerRawBytePower: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_raw_byte_power",
				Help:      "return miner raw byte power(byte).",
			}, []string{"actor_id", "tag"}),
		MinerQualityAdjPower: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_quality_adj_power",
				Help:      "return miner quality adj power(byte).",
			}, []string{"actor_id", "tag"}),
		MinerSectorSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "miner_sector_size",
				Help:      "return miner sector size(byte).",
			}, []string{"actor_id", "tag", "has_min_power"}),
		ActorBalance: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "actor_balance",
				Help:      "return actor balance(fil).",
			}, []string{"actor_id", "key", "tag", "actor_type", "miner_id"}),
		LockedBalance: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "locked_balance",
				Help:      "return locked balance(fil).",
			}, []string{"actor_id", "tag", "locked_type", "miner_id"}),
		native: data,
	}, nil

}
