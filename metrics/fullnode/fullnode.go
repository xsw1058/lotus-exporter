package fullnode

import (
	"context"
	"errors"
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	lotusapi "github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/xsw1058/lotus-exporter/config"
	"github.com/xsw1058/lotus-exporter/metrics"
	"github.com/xsw1058/lotus-exporter/metrics/filfox"
	"net/http"
	"sync"
	"time"
)

var (
	log = logging.Logger("full_node")
)

const (
	NameSpace      = "lotus"
	InfoKey        = "info"
	SyncKey        = "sync"
	DeadlinesKey   = "deadline"
	MessagePoolKey = "mpool"
	ActorKey       = "actor"
	SectorKey      = "sector"
	TransferKey    = "transfer"
)

type chainInfo struct {
	sync.Mutex
	// baseTimestamp 基准时间戳, 用于和区块高度进行匹配
	baseTimestamp float64

	// baseTimestamp 基准高度, 用于和区块时间戳进行匹配
	baseEpoch float64

	// chainHead cmd: lotus chain head
	chainHead *types.TipSet

	// networkVersion the network version. cmd: lotus state network-version
	networkVersion apitypes.NetworkVersion

	// ciDs the built-in actor bundle manifest ID & system actor cids . cmd: lotus state actor-cids
	ciDs map[string]cid.Cid

	// 保存关注的地址
	attentionActor map[address.Address]ActorBase

	// 保存地址的别名信息
	ActorInfo map[ActorBase]string
}

type FullNode struct {
	conf           config.LotusOpt
	reg            metrics.Register
	waitingInitAPI bool
	ctx            context.Context
	api            *lotusapi.FullNodeStruct
	closer         jsonrpc.ClientCloser
	chain          *chainInfo
	transferDetail map[ActorBase]filfox.Transfers
}

func MustNewFullNode(ctx context.Context, opt config.LotusOpt, reg metrics.Register) *FullNode {
	token := opt.LotusToken
	addr := opt.LotusAddress
	headers := http.Header{"Authorization": []string{"Bearer " + token}}

	var Lotus = &FullNode{
		conf:           opt,
		reg:            reg,
		waitingInitAPI: false,
		ctx:            ctx,
		api:            nil,
		closer:         nil,
		chain:          nil,
		transferDetail: make(map[ActorBase]filfox.Transfers),
	}

	var api lotusapi.FullNodeStruct

	closer, err := jsonrpc.NewMergeClient(ctx, "ws://"+addr+"/rpc/v0", "Filecoin", []interface{}{&api.Internal, &api.CommonStruct.Internal}, headers)
	if err != nil {
		// TODO
		log.Errorln(err)
		Lotus.waitingInitAPI = true
		go Lotus.backgroundInit()
		return Lotus
	}
	Lotus.api = &api
	Lotus.closer = closer
	Lotus.init()
	as := Lotus.getAllAttentionActor()
	Lotus.RegisterAttentionActor(as)
	log.Infoln("lotus api init success.")
	return Lotus
}

func (l *FullNode) getAttentionActor() map[address.Address]ActorBase {
	return l.chain.attentionActor
}

func (l *FullNode) RegisterCollectors() {
	l.reg.RegisterHandler(InfoKey, metrics.RightNowScan, l)
	l.reg.RegisterHandler(SyncKey, metrics.RightNowScan, l)
	l.reg.RegisterHandler(MessagePoolKey, metrics.RightNowScan, l)

	l.reg.RegisterHandler(ActorKey, metrics.EachEpochScan, l)
	l.reg.RegisterHandler(TransferKey, metrics.EachEpochScan, l)
	if l.conf.EnableDeadlinesCollector {
		l.reg.RegisterHandler(DeadlinesKey, metrics.EachEpochScan, l)
	}

	if l.conf.EnableSectorCollector {
		l.reg.RegisterHandler(SectorKey, metrics.EachDeadlineScan, l)
	}
}

func (l *FullNode) backgroundInit() {
	defer log.Infoln("lotus api init success.")
	t := time.NewTicker(time.Second * 10)
	for {
		select {
		case <-t.C:
			var api lotusapi.FullNodeStruct
			headers := http.Header{"Authorization": []string{"Bearer " + l.conf.LotusToken}}
			closer, err := jsonrpc.NewMergeClient(l.ctx, "ws://"+l.conf.LotusAddress+"/rpc/v0", "Filecoin", []interface{}{&api.Internal, &api.CommonStruct.Internal}, headers)
			if err != nil {
				log.Errorln(err)
				continue
			}
			l.api = &api
			l.closer = closer
			l.init()
			as := l.getAllAttentionActor()
			l.RegisterAttentionActor(as)
			l.waitingInitAPI = false
			return
		}
	}
}

func (l *FullNode) init() {

	chainHead, err := l.api.ChainHead(l.ctx)
	if err != nil {
		log.Errorln(err)
	}

	networkVersion, err := l.api.StateNetworkVersion(l.ctx, chainHead.Key())
	if err != nil {
		log.Errorln(err)
	}

	ciDs, err := l.api.StateActorCodeCIDs(l.ctx, networkVersion)
	if err != nil {
		log.Errorln(err)
	}
	log.Debugw("StateActorCodeCIDs result", "network", networkVersion, "cids", ciDs)
	walletList, err := l.api.WalletList(l.ctx)
	if err != nil {
		log.Errorln(err)
	}

	var wMap = make(map[address.Address]string)
	for _, w := range walletList {
		wMap[w] = "local"
	}

	chain := chainInfo{
		Mutex:          sync.Mutex{},
		baseTimestamp:  1598306400,
		baseEpoch:      0,
		chainHead:      chainHead,
		networkVersion: networkVersion,
		ciDs:           ciDs,
		attentionActor: make(map[address.Address]ActorBase),
		ActorInfo:      make(map[ActorBase]string),
	}
	l.chain = &chain
}

func (l *FullNode) CreateCollector(handler string) (metrics.LotusCollector, error) {
	if l.waitingInitAPI {
		return nil, errors.New("wait for lotus api init ")
	}
	switch handler {
	case InfoKey:
		info, err := l.UpdateInfo()
		if err != nil {
			return nil, err
		}
		return NewInfoCollector(info)
	case SyncKey:
		data, err := l.ChainSync()
		if err != nil {
			return nil, err
		}
		return NewSyncCollector(data)
	case DeadlinesKey:
		data, err := l.UpdateDeadlines()
		if err != nil {
			return nil, err
		}
		return NewDeadlinesCollector(data)
	case MessagePoolKey:
		data, err := l.UpdateMessagePool()
		if err != nil {
			return nil, err
		}
		return NewMPoolCollector(data)
	case ActorKey:
		data, err := l.GetActorMetric()
		if err != nil {
			return nil, err
		}
		return NewActorInfo(data)
	case SectorKey:
		data, err := l.sectorInfo()
		if err != nil {
			return nil, err
		}
		return NewSectorMetrics(data)
	case TransferKey:
		return l.TransactionsSummary()
	}
	return nil, errors.New(fmt.Sprintf("not found handler %v", handler))
}
