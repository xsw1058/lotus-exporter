package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/build"
	"github.com/xsw1058/lotus-exporter/config"
	"golang.org/x/xerrors"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	lotusapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	lminer "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

func main() {
	log.SetFlags(23)
	opt, err := DefaultOpt()
	if err != nil {
		log.Fatalln(err)
	}
	if opt.DebugMode {
		log.Println(opt)
	}

	ctx := context.Background()
	lotus, closer, err := NewLotus(ctx, *opt)
	if err != nil {
		log.Fatalln(err)
	}
	defer closer()

	err = lotus.CheckExpire()
	if err != nil {
		log.Fatalln(err)
	}
}

type Opt struct {
	MinerID          string
	LotusToken       string
	LotusAddress     string
	RemainLifeDay    int64
	ExtendDay        int64
	ShowSectorDetail bool
	Silence          bool
	DebugMode        bool
	NewExpiration    time.Time
}

func DefaultOpt() (*Opt, error) {
	var token, lotusAddr string
	lotusToken := flag.String("token", "", "LotusToken")
	lotusAddress := flag.String("address", "", "LotusAddress")
	minerID := flag.String("m", "f01098119", "minerID")
	newExpirationStr := flag.String("e", "2022-12-22", "NewExpiration")
	remainLifeDay := flag.Int64("r", 7, "RemainLifeDay")
	extendDay := flag.Int64("i", 180, "ExtendDay")
	showSectorDetail := flag.Bool("d", false, "ShowSectorDetail")
	silence := flag.Bool("s", true, "silence")
	debugMode := flag.Bool("debug", false, "debug")
	flag.Parse()

	if *lotusToken == "" || *lotusAddress == "" {
		a, t, err := config.GetApiInfoFromEnv("FULLNODE_API_INFO")
		if err != nil {
			return nil, xerrors.Errorf("get api from env(FULLNODE_API_INFO): %w", err)
		}
		token = t
		lotusAddr = a
	} else {
		token = *lotusToken
		lotusAddr = *lotusAddress
	}

	var newExpiration = time.Time{}
	l, _ := time.LoadLocation("Local")

	t, err := time.ParseInLocation("2006-01-02", *newExpirationStr, l)
	if err == nil {
		newExpiration = t
	}

	return &Opt{
		MinerID:          *minerID,
		LotusToken:       token,
		LotusAddress:     lotusAddr,
		RemainLifeDay:    *remainLifeDay,
		ExtendDay:        *extendDay,
		NewExpiration:    newExpiration,
		ShowSectorDetail: *showSectorDetail,
		Silence:          *silence,
		DebugMode:        *debugMode,
	}, nil
}

type Lotus struct {
	api           lotusapi.FullNodeStruct
	opt           Opt
	ctx           context.Context
	newExpiration uint64
}

func NewLotus(ctx context.Context, opt Opt) (*Lotus, jsonrpc.ClientCloser, error) {
	token := opt.LotusToken
	addrLotus := opt.LotusAddress
	headers := http.Header{"Authorization": []string{"Bearer " + token}}

	var api lotusapi.FullNodeStruct

	closer, err := jsonrpc.NewMergeClient(ctx, "ws://"+addrLotus+"/rpc/v0", "Filecoin", []interface{}{&api.Internal, &api.CommonStruct.Internal}, headers)
	if err != nil {
		return nil, nil, xerrors.Errorf("json rpc client from %s: %w", addrLotus, err)
	}

	return &Lotus{api: api, opt: opt, ctx: ctx, newExpiration: uint64(0)}, closer, nil
}

func (l *Lotus) CheckExpire() error {
	addr, err := address.NewFromString(l.opt.MinerID)
	if err != nil {
		return xerrors.Errorf("convert string to address %s: %w", l.opt.MinerID, err)
	}

	head, err := l.api.ChainHead(l.ctx)
	if err != nil {
		return xerrors.Errorf("get api chain head: %w", err)
	}
	currEpoch := head.Height()

	nv, err := l.api.StateNetworkVersion(l.ctx, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("get api network version: %w", err)
	}

	sectors, err := l.api.StateMinerActiveSectors(l.ctx, addr, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("get api miner active sectors %v: %w", addr.String(), err)
	}

	var sectorsNumbers []uint64
	n := 0
	for _, s := range sectors {

		if s.Expiration-currEpoch <= abi.ChainEpoch(builtin.EpochsInDay*l.opt.RemainLifeDay) {
			sectorsNumbers = append(sectorsNumbers, uint64(s.SectorNumber))
			sectors[n] = s
			n++
		}
	}

	sectors = sectors[:n]

	sort.Slice(sectors, func(i, j int) bool {
		if sectors[i].Expiration == sectors[j].Expiration {
			return sectors[i].SectorNumber < sectors[j].SectorNumber
		}
		return sectors[i].Expiration < sectors[j].Expiration
	})
showDetail:
	if l.opt.ShowSectorDetail {
		tw := tablewriter.New(
			tablewriter.Col("ID"),
			tablewriter.Col("SealProof"),
			tablewriter.Col("InitialPledge"),
			tablewriter.Col("Activation"),
			tablewriter.Col("Expiration"),
			tablewriter.Col("MaxExpiration"),
			tablewriter.Col("MaxExtendNow"))
		for _, sector := range sectors {
			MaxExpiration := sector.Activation + policy.GetSectorMaxLifetime(sector.SealProof, nv)
			MaxExtendNow := currEpoch + policy.GetMaxSectorExpirationExtension()

			if MaxExtendNow > MaxExpiration {
				MaxExtendNow = MaxExpiration
			}

			tw.Write(map[string]interface{}{
				"ID":            sector.SectorNumber,
				"SealProof":     sector.SealProof,
				"InitialPledge": types.FIL(sector.InitialPledge).Short(),
				"Activation":    sector.Activation,
				"Expiration":    sector.Expiration,
				"MaxExpiration": MaxExpiration,
				"MaxExtendNow":  MaxExtendNow,
			})
		}
		err := tw.Flush(os.Stdout)
		if err != nil {
			return xerrors.Errorf("table writer flush: %w", l.opt.MinerID, err)
		}
	}
	if len(sectorsNumbers) == 0 {
		fmt.Println("no expire sector found")
		return nil
	}
	fmt.Printf("len:%v, numbers:%v\n", len(sectorsNumbers), sectorsNumbers)
	if !l.opt.Silence {

		var res int
		fmt.Println("input... 1: continue; 2:show detail; 3: new expiration;other:exit")
		fmt.Scan(&res)
		switch res {
		case 1:
			break
		case 2:
			l.opt.ShowSectorDetail = true
			goto showDetail
		case 3:
			var newExpiration uint64
			_, err := fmt.Scan(&newExpiration)
			if err != nil {
				fmt.Println("input invalid exit....")
				return nil
			}
			l.newExpiration = newExpiration
		default:
			return nil
		}
	}
	return l.Extend(sectorsNumbers)
}

func (l *Lotus) Extend(sectorsNum []uint64) error {

	maddr, err := address.NewFromString(l.opt.MinerID)
	if err != nil {
		return xerrors.Errorf("convert string to address %s: %w", l.opt.MinerID, err)
	}
	// TODO 解析NewExpiration
	var newExpiration abi.ChainEpoch

	chainHead, err := l.api.ChainHead(l.ctx)
	if err != nil {
		return xerrors.Errorf("getting chain head: %w", err)
	}
	currEpoch := chainHead.Height()

	now := time.Now()

	switch {
	case l.newExpiration > uint64(currEpoch):
		newExpiration = abi.ChainEpoch(l.newExpiration)
	case now.After(l.opt.NewExpiration):
		newExpiration = currEpoch + abi.ChainEpoch(l.opt.ExtendDay*builtin.EpochsInDay)
	default:
		a := builtin.EpochsInHour * l.opt.NewExpiration.Sub(now).Hours()
		newExpiration = currEpoch + abi.ChainEpoch(int64(a))
	}

	t := (uint64(newExpiration-currEpoch) * build.BlockDelaySecs) + chainHead.Blocks()[0].Timestamp
	atTime := time.Unix(int64(t), 0)
	fmt.Printf("new expiration:%v at:%v\n", newExpiration, atTime.Local().Format("2006-01-02 15:04:05"))
	if !l.opt.Silence {
		log.Println("input any to continue")
		var a string
		fmt.Scan(&a)
	}

	var params []miner.ExtendSectorExpirationParams

	sectors := map[lminer.SectorLocation][]uint64{}

	for _, id := range sectorsNum {

		p, err := l.api.StateSectorPartition(l.ctx, maddr, abi.SectorNumber(id), types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting sector location for sector %d: %w", id, err)
		}

		if p == nil {
			return xerrors.Errorf("sector %d not found in any partition", id)
		}

		sectors[*p] = append(sectors[*p], id)
	}

	p := miner.ExtendSectorExpirationParams{}
	for sectorLocation, numbers := range sectors {

		p.Extensions = append(p.Extensions, miner.ExpirationExtension{
			Deadline:      sectorLocation.Deadline,
			Partition:     sectorLocation.Partition,
			Sectors:       bitfield.NewFromSet(numbers),
			NewExpiration: newExpiration,
		})
	}

	params = append(params, p)

	if len(params) == 0 {
		fmt.Println("nothing to extend")
		return nil
	}

	mi, err := l.api.StateMinerInfo(l.ctx, maddr, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getting miner info: %w", err)
	}

	for i := range params {
		sp, aerr := actors.SerializeParams(&params[i])
		if aerr != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		smsg, err := l.api.MpoolPushMessage(l.ctx, &types.Message{
			From:   mi.Worker,
			To:     maddr,
			Method: builtin.MethodsMiner.ExtendSectorExpiration,

			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push message: %w", err)
		}

		fmt.Println(smsg.Cid())
	}

	return nil
}
