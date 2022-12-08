package main

import (
	"context"
	"fmt"
	jsonrpc "github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	lotusapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/hako/durafmt"
	cbor "github.com/ipfs/go-ipld-cbor"
	"log"
	"net/http"
	"time"
)

type FullNode struct {
	ctx    context.Context
	api    *lotusapi.FullNodeStruct
	closer jsonrpc.ClientCloser
}

func NewFullNode(ctx context.Context, token string, addr string) *FullNode {

	fullNodeStruct, closer, err := initFullNodeOnce(ctx, token, addr)
	if err != nil {
		return nil
	}
	return &FullNode{
		ctx:    ctx,
		api:    fullNodeStruct,
		closer: closer,
	}
}

func (f *FullNode) producedBlocks(p1 abi.ChainEpoch) error {
	var err error
	//maddr, err := address.NewFromString("f01098119")
	//if err != nil {
	//	return err
	//}

	head, err := f.api.ChainHead(f.ctx)
	if err != nil {
		return err
	}
	//head, err = f.api.ChainGetTipSetByHeight(f.ctx, p1, head.Key())
	//if err != nil {
	//	return err
	//}
	tbs := blockstore.NewTieredBstore(blockstore.NewAPIBlockstore(f.api), blockstore.NewMemory())

	//tty := isatty.IsTerminal(os.Stderr.Fd())

	ts := head
	fmt.Printf(" Epoch   | Block ID                                                       | Reward\n")
	//for count > 0 {
	tsk := ts.Key()
	bhs := ts.Blocks()
	for _, bh := range bhs {
		//if ctx.Err() != nil {
		//	return ctx.Err()
		//}

		//if bh.Miner == maddr {
		//if tty {
		//	_, _ = fmt.Fprint(os.Stderr, "\r\x1b[0K")
		//}

		rewardActor, err := f.api.StateGetActor(f.ctx, reward.Address, tsk)
		if err != nil {
			return err
		}

		rewardActorState, err := reward.Load(adt.WrapStore(f.ctx, cbor.NewCborStore(tbs)), rewardActor)
		if err != nil {
			return err
		}
		blockReward, err := rewardActorState.ThisEpochReward()
		if err != nil {
			return err
		}

		minerReward := types.BigDiv(types.BigMul(types.NewInt(uint64(bh.ElectionProof.WinCount)),
			blockReward), types.NewInt(uint64(builtin.ExpectedLeadersPerEpoch)))

		fmt.Printf("%v: %8d | %s | %s\n", bh.Miner.String(), ts.Height(), bh.Cid(), types.FIL(minerReward))
		//count--
		//} else if tty && bh.Height%120 == 0 {
		//	_, _ = fmt.Fprintf(os.Stderr, "\r\x1b[0KChecking epoch %s", EpochTime(head.Height(), bh.Height))
		//}
	}
	//tsk = ts.Parents()
	//ts, err = f.api.ChainGetTipSet(f.ctx, tsk)
	//if err != nil {
	//	return err
	//}
	//}

	//if tty {
	//	_, _ = fmt.Fprint(os.Stderr, "\r\x1b[0K")
	//}

	return nil
}

func EpochTime(curr, e abi.ChainEpoch) string {
	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s ago)", e, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(curr-e))).LimitFirstN(2))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return fmt.Sprintf("%d (in %s)", e, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(e-curr))).LimitFirstN(2))
	}

	panic("math broke")
}

func initFullNodeOnce(ctx context.Context, token string, addr string) (*lotusapi.FullNodeStruct, jsonrpc.ClientCloser, error) {
	headers := http.Header{"Authorization": []string{"Bearer " + token}}

	var api lotusapi.FullNodeStruct
	closer, err := jsonrpc.NewMergeClient(ctx, "ws://"+addr+"/rpc/v0", "Filecoin", []interface{}{&api.Internal, &api.CommonStruct.Internal}, headers)
	if err != nil {
		return nil, nil, err
	}
	//chainHead, err := api.ChainHead(ctx)
	//if err != nil {
	//	return nil, nil,  err
	//}
	//
	//networkVersion, err := api.StateNetworkVersion(ctx, chainHead.Key())
	//if err != nil {
	//	return nil, nil, err
	//}
	//
	//ciDs, err := api.StateActorCodeCIDs(ctx, networkVersion)
	//if err != nil {
	//	return nil, nil, err
	//}
	//
	//walletList, err := api.WalletList(ctx)
	//if err != nil {
	//	return nil, nil, err
	//}
	//
	//var wMap = make(map[address.Address]string)
	//for _, w := range walletList {
	//	wMap[w] = "local"
	//}

	return &api, closer, nil
}

func main() {
	var token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJyZWFkIiwid3JpdGUiLCJzaWduIiwiYWRtaW4iXX0.6vJdRkA_9KmxI6-okljIX9OzHcb7sgYP6O7erqqUzao"
	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*10)
	defer cancelFunc()
	node := NewFullNode(ctx, token, "121.204.248.35:11234")

	err := node.producedBlocks(2383681)
	if err != nil {
		log.Println(err)
	}
}
