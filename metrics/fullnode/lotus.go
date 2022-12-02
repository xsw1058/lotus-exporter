package fullnode

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	lotusapi "github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"github.com/xsw1058/lotus-exporter/config"
	"github.com/xsw1058/lotus-exporter/metrics"
	"log"
	"net/http"
	"sync"
)

type ChainMetric struct {
	Version        string
	ChainHeight    float64
	BaseFee        float64
	LocalTime      float64
	NetworkVersion float64
}

//var (
//	log = logging.Logger("full_node_metric")
//)

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

	// followAddress 关注的地址. 用于检测消息池中是否有该地址发送的消息.
	followAddress map[address.Address]string
}

type FullNode struct {
	mem    metrics.StorageAndLoader
	ctx    context.Context
	api    lotusapi.FullNodeStruct
	closer jsonrpc.ClientCloser
	chain  *chainInfo
}

func NewFullNode(ctx context.Context, opt config.LotusOpt, mem metrics.StorageAndLoader) (*FullNode, error) {
	var token = opt.LotusToken
	var addr = opt.LotusAddress
	log.Println(addr, token)
	headers := http.Header{"Authorization": []string{"Bearer " + token}}

	var api lotusapi.FullNodeStruct
	closer, err := jsonrpc.NewMergeClient(ctx, "ws://"+addr+"/rpc/v0", "Filecoin", []interface{}{&api.Internal, &api.CommonStruct.Internal}, headers)
	if err != nil {
		return nil, err
	}
	chainHead, err := api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	networkVersion, err := api.StateNetworkVersion(ctx, chainHead.Key())
	if err != nil {
		return nil, err
	}

	ciDs, err := api.StateActorCodeCIDs(ctx, networkVersion)
	if err != nil {
		return nil, err
	}

	walletList, err := api.WalletList(ctx)
	if err != nil {
		return nil, err
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
		followAddress:  wMap,
	}

	return &FullNode{
		mem:    mem,
		ctx:    ctx,
		api:    api,
		closer: closer,
		chain:  &chain,
	}, nil
}

// Update lotus api 查询逻辑
func (l *FullNode) Update(storage metrics.Storage) error {
	l.chain.Lock()
	defer l.chain.Unlock()

	// ChainHead 是每次链状态更新都要随着更新的.
	chainHead, err := l.api.ChainHead(l.ctx)
	if err != nil {
		return err
	}
	l.chain.chainHead = chainHead
	l.chain.baseEpoch = float64(chainHead.Blocks()[0].Height)
	l.chain.baseTimestamp = float64(chainHead.Blocks()[0].Timestamp)

	// StateNetworkVersion 由于可能存在网络版本升级,所以要随着链状态的更新而更新的.
	networkVersion, err := l.api.StateNetworkVersion(l.ctx, l.chain.chainHead.Key())
	if err != nil {
		return err
	}

	if l.chain.networkVersion != networkVersion {
		l.chain.networkVersion = networkVersion
		// StateActorCodeCIDs 是随着网络版本的更新而更新的.
		ciDs, err := l.api.StateActorCodeCIDs(l.ctx, networkVersion)
		if err != nil {
			return err
		}
		l.chain.ciDs = ciDs
	}

	lotusAPIVersion, err := l.api.Version(l.ctx)
	if err != nil {
		return err
	}

	var i ChainMetric
	i.LocalTime = float64(chainHead.Blocks()[0].Timestamp)
	i.BaseFee = float64(chainHead.Blocks()[0].ParentBaseFee.Int64())
	i.ChainHeight = float64(chainHead.Height())
	i.Version = lotusAPIVersion.String()
	i.NetworkVersion = float64(networkVersion)
	storage.Store(metrics.InfoKey, i)
	return nil
}
