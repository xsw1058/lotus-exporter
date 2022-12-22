package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"github.com/filecoin-project/go-jsonrpc"
	lotusapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/xsw1058/lotus-exporter/config"
	"net/http"
)

func sectorInfo() {
	log := logging.Logger("sector")
	opt := config.DefaultOpt()
	token := opt.LotusToken
	addrLotus := opt.LotusAddress
	headers := http.Header{"Authorization": []string{"Bearer " + token}}
	ctx := context.Background()

	var api lotusapi.FullNodeStruct

	closer, err := jsonrpc.NewMergeClient(ctx, "ws://"+addrLotus+"/rpc/v0", "Filecoin", []interface{}{&api.Internal, &api.CommonStruct.Internal}, headers)
	if err != nil {
		log.Fatalln(err)
	}
	defer closer()

	currChainHead, err := api.ChainHead(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	log.Infow("current chain", "height", currChainHead.Height(), "tipSet Key", currChainHead.Key())

	// 获取指定高度的TipSet
	//chainHead, err := api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(145000), currChainHead.Key())
	//if err != nil {
	//	log.Errorln(err)
	//	return
	//}

	//nv, err := api.StateNetworkVersion(ctx, currChainHead.Key())
	//if err != nil {
	//	log.Fatalln(err)
	//}
	c, err := cid.Parse("bafy2bzaceczogwlxvgt4uzhwop32rfe3eszwgdj3yanpkug7m2xxk2ynniebs")
	if err != nil {
		log.Fatalln(err)
	}
	//msg, err := api.StateWaitMsg(ctx, c, 42, currChainHead.Height()+4, true)
	//if err != nil {
	//	log.Fatalln(err)
	//}
	//log.Infoln(msg)
	searchMsg, err := api.StateSearchMsg(ctx, currChainHead.Key(), c, currChainHead.Height(), true)
	if err != nil {
		log.Fatalln(err)
	}
	log.Infoln(searchMsg)

}

func GetBytes(key interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(key)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func main() {
	sectorInfo()
}

func DecodeMessage() {
	log := logging.Logger("main")
	opt := config.DefaultOpt()
	token := opt.LotusToken
	addrLotus := opt.LotusAddress
	headers := http.Header{"Authorization": []string{"Bearer " + token}}
	ctx := context.Background()

	var api lotusapi.FullNodeStruct

	closer, err := jsonrpc.NewMergeClient(ctx, "ws://"+addrLotus+"/rpc/v0", "Filecoin", []interface{}{&api.Internal, &api.CommonStruct.Internal}, headers)
	if err != nil {
		log.Errorln(err)
		return
	}
	defer closer()

	chainHead, err := api.ChainHead(ctx)
	if err != nil {
		log.Errorln(err)
		return
	}

	//c, err := cid.Decode("bafy2bzaceczwtnzultuv7635czkfmmmf2wzbvmzaypur2bomyml3uhh6kpm4q") //	Propose
	//c, err := cid.Decode("bafy2bzaceamafcylcbvnvs2ciduwypzh4cm7l5bg43fop7uyrexsuzd6hoo6g") //	Approve

	//c, err := cid.Decode("bafy2bzaceacmpx4x2zyfb65y3g5e2ebjkivozptbtv4kpkzs7gpmbl7kpqujw") //	Approve
	c, err := cid.Decode("bafy2bzacecjsyair3ilvcushngk72na7ls2nehpe5te4g7tl2a47fbloc2ste") //	Propose
	if err != nil {
		log.Errorln(err)
		return
	}

	message, err := api.ChainGetMessage(ctx, c)
	if err != nil {
		log.Errorln(err)
		return
	}

	var pro multisig.ProposeParams
	if err := pro.UnmarshalCBOR(bytes.NewReader(message.Params)); err != nil {
		log.Infof("failed to unmarshal propose return value: %s", err)
		return
	}
	log.Infoln(pro)

	msg, err := api.StateSearchMsg(ctx, chainHead.Key(), c, chainHead.Height(), true)
	if err != nil {
		log.Errorln(err)
		return
	}

	log.Infoln(msg)

	if msg.Receipt.ExitCode.IsError() {
		log.Errorln(err)
		return
	}

	var retval multisig.ProposeReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(msg.Receipt.Return)); err != nil {
		log.Infof("failed to unmarshal propose return value: %s", err)
		return
	}
	log.Infoln(retval)
}
