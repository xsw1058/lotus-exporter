package main

import (
	"context"
	"errors"
	"flag"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
	logging "github.com/ipfs/go-log/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"lotus-exporter/metrics"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	logger                                                   = logging.Logger("main")
	deadlineLabel                                            = []string{"api", "miner_id", "miner_tag", "index"}
	fullNodeAddress, fullNodeToken, minerAddress, minerToken string
	actorAddresses                                           = make(map[address.Address]string)
	walletAddresses                                          = make(map[address.Address]string)
	FullNodeCtx                                              = context.Background()
	APIFullNode                                              = api.FullNodeStruct{}
	BaseTimestamp                                            = float64(1598306400)
	BaseEpoch                                                = float64(0)
)

const (
	namespace = "lotus"
)

type FullNodeMetrics struct {
	sync.Mutex
	ScrapeDuration *prometheus.GaugeVec
	CurrentTime    *prometheus.GaugeVec
	// lotus
	CurrentEpoch                  *prometheus.GaugeVec
	BaseFee                       *prometheus.GaugeVec
	FullNodeInfo                  *prometheus.GaugeVec
	FullNodeSync                  *prometheus.GaugeVec
	MinerDeadlineOpenTime         *prometheus.GaugeVec
	MinerDeadlineSectorsRecovery  *prometheus.GaugeVec
	MinerDeadlineSectorsFaulty    *prometheus.GaugeVec
	MinerDeadlineSectorsAll       *prometheus.GaugeVec
	MinerDeadlinePartitions       *prometheus.GaugeVec
	MinerDeadlinePartitionsProven *prometheus.GaugeVec
	MinerDeadlineOpenEpoch        *prometheus.GaugeVec
	MinerDeadlinesInfo            *prometheus.GaugeVec
	MinerRawBytePower             *prometheus.GaugeVec
	MinerQualityAdjPower          *prometheus.GaugeVec
	MinerSectorSize               *prometheus.GaugeVec
	ActorBalance                  *prometheus.GaugeVec
	// TODO miner
	//version        *prometheus.GaugeVec
	//workerName     *prometheus.GaugeVec
	//sealingJob     *prometheus.GaugeVec
	//localStorage   *prometheus.GaugeVec
	//sectorsSummary *prometheus.GaugeVec
}

func NewFullNode() *FullNodeMetrics {
	return &FullNodeMetrics{
		Mutex: sync.Mutex{},
		ScrapeDuration: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "scrape_duration",
				Help:      "return collector duration time(seconds).",
			}, []string{"api", "collector"}),
		CurrentTime: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "current_time",
				Help:      "return current timestamp(unix).",
			}, []string{"api"}),
		CurrentEpoch: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "current_epoch",
				Help:      "return lotus current height.",
			}, []string{"api"}),
		BaseFee: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "base_fee",
				Help:      "return lotus base fee(attoFil).",
			}, []string{"api"}),
		FullNodeInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "info",
				Help:      "return lotus network version.",
			}, []string{"api", "version"}),
		FullNodeSync: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "sync",
				Help:      "return full node sync diff.",
			}, []string{"api", "worker_id", "status"}),
		MinerDeadlineOpenTime: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "miner_deadline_open_time",
				Help:      "return miner deadline open timestamp(unix).",
			}, deadlineLabel),
		MinerDeadlineSectorsRecovery: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "miner_deadline_sector_recovery",
				Help:      "return miner deadline recovery sector number.",
			}, deadlineLabel),
		MinerDeadlineSectorsFaulty: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "miner_deadline_sector_faulty",
				Help:      "return miner deadline faulty sector number.",
			}, deadlineLabel),
		MinerDeadlineSectorsAll: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "miner_deadline_sector_all",
				Help:      "return miner deadline all sector number.",
			}, deadlineLabel),
		MinerDeadlinePartitions: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "miner_deadline_partitions",
				Help:      "return miner deadline partitions number.",
			}, deadlineLabel),
		MinerDeadlinePartitionsProven: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "miner_deadline_partitions_proven",
				Help:      "return miner deadline proven partitions number.",
			}, deadlineLabel),
		MinerDeadlineOpenEpoch: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "miner_deadline_open_epoch",
				Help:      "return miner deadline open epoch.",
			}, deadlineLabel),
		MinerDeadlinesInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "miner_deadline_info",
				Help:      "return miner current deadline cost(-1:proven,0;unknown,other:cost height).",
			}, []string{"api", "miner_id", "miner_tag", "period_start", "index", "open_epoch"}),
		MinerRawBytePower: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "miner_raw_byte_power",
				Help:      "return miner raw byte power(byte).",
			}, []string{"api", "miner_id", "miner_tag"}),
		MinerQualityAdjPower: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "miner_quality_adj_power",
				Help:      "return miner quality adj power(byte).",
			}, []string{"api", "miner_id", "miner_tag", "has_min_power"}),
		MinerSectorSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "miner_sector_size",
				Help:      "return miner sector size(byte).",
			}, []string{"api", "miner_id", "miner_tag"}),
		ActorBalance: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "actor_balance",
				Help:      "return actor balance(fil).",
			}, []string{"api", "miner_id", "address", "tag", "account_id", "miner_tag"}),
	}
}

func (f *FullNodeMetrics) Describe(ch chan<- *prometheus.Desc) {
	f.CurrentTime.Describe(ch)
	f.ScrapeDuration.Describe(ch)
	f.CurrentEpoch.Describe(ch)
	f.BaseFee.Describe(ch)
	f.FullNodeInfo.Describe(ch)
	f.FullNodeSync.Describe(ch)
	f.MinerDeadlineOpenTime.Describe(ch)
	f.MinerDeadlineSectorsRecovery.Describe(ch)
	f.MinerDeadlineSectorsFaulty.Describe(ch)
	f.MinerDeadlineSectorsAll.Describe(ch)
	f.MinerDeadlinePartitions.Describe(ch)
	f.MinerDeadlinePartitionsProven.Describe(ch)
	f.MinerDeadlineOpenEpoch.Describe(ch)
	f.MinerDeadlinesInfo.Describe(ch)
	f.MinerRawBytePower.Describe(ch)
	f.MinerQualityAdjPower.Describe(ch)
	f.MinerSectorSize.Describe(ch)
	f.ActorBalance.Describe(ch)
}

func (f *FullNodeMetrics) Collect(ch chan<- prometheus.Metric) {
	f.Lock()
	defer f.Unlock()
	f.CurrentTime.Reset()
	f.ScrapeDuration.Reset()
	f.CurrentEpoch.Reset()
	f.BaseFee.Reset()
	f.FullNodeInfo.Reset()
	f.FullNodeSync.Reset()
	f.MinerDeadlineOpenTime.Reset()
	f.MinerDeadlineSectorsRecovery.Reset()
	f.MinerDeadlineSectorsFaulty.Reset()
	f.MinerDeadlineSectorsAll.Reset()
	f.MinerDeadlinePartitions.Reset()
	f.MinerDeadlinePartitionsProven.Reset()
	f.MinerDeadlineOpenEpoch.Reset()
	f.MinerDeadlinesInfo.Reset()
	f.MinerRawBytePower.Reset()
	f.MinerQualityAdjPower.Reset()
	f.MinerSectorSize.Reset()
	f.ActorBalance.Reset()
	// start
	globalStart := time.Now()
	headers := http.Header{"Authorization": []string{"Bearer " + fullNodeToken}}
	closer, err := jsonrpc.NewMergeClient(FullNodeCtx, "ws://"+fullNodeAddress+"/rpc/v0", "Filecoin", []interface{}{&APIFullNode.Internal, &APIFullNode.CommonStruct.Internal}, headers)
	if err != nil {
		logger.Warnf("connecting with lotus failed: %s", err)
		return
	}
	defer closer()
	// 获取链头部信息
	ChainHead, err := APIFullNode.ChainHead(FullNodeCtx)
	if err != nil {
		logger.Warnf("get full node chain head: %v", err)
		return
	}

	// 更新基准Epoch和基准时间戳。
	if len(ChainHead.Blocks()) > 0 {
		BaseEpoch = float64(ChainHead.Blocks()[0].Height)
		BaseTimestamp = float64(ChainHead.Blocks()[0].Timestamp)
		f.BaseFee.WithLabelValues(fullNodeAddress).Set(float64(ChainHead.Blocks()[0].ParentBaseFee.Int64()))
		f.CurrentEpoch.WithLabelValues(fullNodeAddress).Set(float64(ChainHead.Height()))
	}

	// #########构建变量完成。
	var wg = sync.WaitGroup{}
	// 每个矿工的算力、各种余额等信息。
	chb := make(chan metrics.ActorInfo, 4)
	// 矿工的deadline信息。
	chd := make(chan metrics.ActorDeadlines, 4)
	// lotus daemon节点的版本、同步状态等。
	chi := make(chan metrics.FullNodeInfo)
	// 外部钱包的余额。
	chw := make(chan metrics.BalanceMetrics, 4)

	wg.Add(1)
	go func() {
		defer wg.Done()
		counter := 0
		for bb := range chb {
			counter++
			// {"api","collector"}
			f.ScrapeDuration.WithLabelValues(fullNodeAddress, "info_"+bb.ActorStr).Set(bb.DurationSeconds)
			// {"api", "miner_id", "tag"}
			f.MinerSectorSize.WithLabelValues(fullNodeAddress, bb.ActorStr, bb.ActorTag).Set(bb.SectorSize)
			// {"api", "miner_id", "tag", "has_min_power"}
			f.MinerQualityAdjPower.WithLabelValues(fullNodeAddress, bb.ActorStr, bb.ActorTag, strconv.Itoa(bb.HasMinPower)).Set(bb.QualityAdjPower)
			// {"api", "miner_id", "tag"}
			f.MinerRawBytePower.WithLabelValues(fullNodeAddress, bb.ActorStr, bb.ActorTag).Set(bb.RawBytePower)
			for _, bls := range bb.Bls {
				// {"api", "miner_id", "address", "tag", "account_id"}
				f.ActorBalance.WithLabelValues(fullNodeAddress, bb.ActorStr, bls.Address, bls.Tag, bls.AccountId, bb.ActorTag).Set(bls.Balance)
			}
		}
		logger.Debugw("receive", "actor info", counter)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counter := 0
		for dd := range chd {
			counter++
			// {"api","collector"}
			f.ScrapeDuration.WithLabelValues(fullNodeAddress, "deadline_"+dd.ActorStr).Set(dd.DurationSeconds)
			// {"api", "miner_id", "tag","period_start", "index","open_epoch"}
			f.MinerDeadlinesInfo.WithLabelValues(fullNodeAddress, dd.ActorStr, dd.ActorTag, dd.PeriodStart, dd.CurrentIdx, dd.CurrentOpenEpoch).Set(dd.CurrentCost)
			for _, dl := range dd.DlsMetrics {
				// {"api", "miner_id", "tag", "index"}
				f.MinerDeadlineOpenEpoch.WithLabelValues(fullNodeAddress, dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.OpenEpoch)
				f.MinerDeadlineOpenTime.WithLabelValues(fullNodeAddress, dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.OpenTime)
				f.MinerDeadlineSectorsRecovery.WithLabelValues(fullNodeAddress, dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.Recovery)
				f.MinerDeadlineSectorsFaulty.WithLabelValues(fullNodeAddress, dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.Faults)
				f.MinerDeadlineSectorsAll.WithLabelValues(fullNodeAddress, dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.Sectors)
				f.MinerDeadlinePartitions.WithLabelValues(fullNodeAddress, dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.Partitions)
				f.MinerDeadlinePartitionsProven.WithLabelValues(fullNodeAddress, dd.ActorStr, dd.ActorTag, dl.Index).Set(dl.Proven)
			}
		}
		logger.Debugw("receive", "deadline", counter)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counter := 0
		for ii := range chi {
			counter++
			f.ScrapeDuration.WithLabelValues(fullNodeAddress, "fullnode_info").Set(ii.DurationSeconds)
			f.FullNodeInfo.WithLabelValues(fullNodeAddress, ii.Version).Set(ii.NetworkVersion)
			for _, sy := range ii.SyncState {
				// {"api","worker_id","status"}
				f.FullNodeSync.WithLabelValues(fullNodeAddress, sy.WorkerID, sy.Status).Set(sy.HeightDiff)
			}
		}
		logger.Debugw("receive", "fullnode_info", counter)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for w := range chw {
			// {"api", "miner_id", "address", "tag", "account_id"}
			f.ActorBalance.WithLabelValues(fullNodeAddress, "external", w.Address, w.Tag, w.AccountId, "unknown").Set(w.Balance)
		}
	}()
	a := metrics.NewFullNodeApi(&APIFullNode, FullNodeCtx, ChainHead.Key())
	go a.GetActorsDeadlinesMetrics(actorAddresses, BaseEpoch, BaseTimestamp, chd)
	go a.GetActorsInfoMetrics(actorAddresses, chb)
	go a.GetExternalWallet(walletAddresses, chw)
	go a.GetFullNodeInfo(chi)

	wg.Wait()
	f.ScrapeDuration.WithLabelValues(fullNodeAddress, "all").Set(time.Now().Sub(globalStart).Seconds())
	f.CurrentTime.WithLabelValues(fullNodeAddress).Set(float64(time.Now().Unix()))
	//end
	f.CurrentTime.Collect(ch)
	f.ScrapeDuration.Collect(ch)
	f.CurrentEpoch.Collect(ch)
	f.BaseFee.Collect(ch)
	f.FullNodeInfo.Collect(ch)
	f.FullNodeSync.Collect(ch)
	f.MinerDeadlineOpenTime.Collect(ch)
	f.MinerDeadlineSectorsRecovery.Collect(ch)
	f.MinerDeadlineSectorsFaulty.Collect(ch)
	f.MinerDeadlineSectorsAll.Collect(ch)
	f.MinerDeadlinePartitions.Collect(ch)
	f.MinerDeadlinePartitionsProven.Collect(ch)
	f.MinerDeadlineOpenEpoch.Collect(ch)
	f.MinerDeadlinesInfo.Collect(ch)
	f.MinerRawBytePower.Collect(ch)
	f.MinerQualityAdjPower.Collect(ch)
	f.MinerSectorSize.Collect(ch)
	f.ActorBalance.Collect(ch)
}

func main() {
	var port string
	flag.StringVar(&port, "p", ":9002", "listen address.")
	flag.Parse()
	fa, ft, err := GetApiInfoFromEnv("FULLNODE_API_INFO")
	if err != nil {
		logger.Warnw("init error", "FULLNODE_API_INFO", err)
	} else {
		fullNodeAddress = fa
		fullNodeToken = ft
	}
	logger.Debugw("init result", "fullNode address", fullNodeAddress, "fullNode token length", len(fullNodeToken))

	ma, mt, err := GetApiInfoFromEnv("MINER_API_INFO")
	if err != nil {
		logger.Warnw("init error", "MINER_API_INFO", err)
	} else {
		minerAddress = ma
		minerToken = mt
	}
	logger.Debugw("init result", "miner address", minerAddress, "miner token length", len(minerToken))

	err = GetAddressFromEnv("MINERS_ID", actorAddresses)
	if err != nil {
		logger.Warnw("init error", "MINERS_ID", err)
	}
	logger.Debugw("init result", "miners", len(actorAddresses))

	err = GetAddressFromEnv("LOTUS_WALLETS", walletAddresses)
	if err != nil {
		logger.Warnw("init error", "LOTUS_WALLETS", err)
	}
	logger.Debugw("init result", "wallets", len(walletAddresses))

	prometheus.MustRegister(NewFullNode())
	logger.Infow("init result", "listen address", port)
	logger.Fatalf("ListenAndServe error: %v", http.ListenAndServe(port, promhttp.Handler()))
}

// GetAddressFromEnv from os env key get lotus address.
//
// Usage: ENV_KEY="<address>:<tag>,<address>:<tag>"
func GetAddressFromEnv(envKey string, actorAddresses map[address.Address]string) error {
	env, ok := os.LookupEnv(envKey)
	if !ok {
		return errors.New("now found")
	}
	s := strings.Split(env, ",")
	for _, w := range s {
		a := strings.Split(w, ":")
		fromString, err := address.NewFromString(a[0])
		if err != nil {
			logger.Warnw("decode addr", a[0], err)
			continue
		}
		if len(a) < 2 {
			actorAddresses[fromString] = "unknown"
		} else {
			actorAddresses[fromString] = a[1]
		}
	}
	if len(actorAddresses) == 0 {
		return errors.New("all address invalid")
	}
	return nil
}

// GetApiInfoFromEnv from os env key get lotus api info.
//
// command: lotus auth api-info --perm xxx
//
// Such as: FULLNODE_API_INFO=<token>:/ip4/<address>/tcp/<port>/http
func GetApiInfoFromEnv(envKey string) (addr, token string, err error) {
	env, ok := os.LookupEnv(envKey)
	if !ok {
		return "", "", errors.New("not found")
	}
	minerAddr := strings.Split(env, "/")
	conn, err := net.DialTimeout("tcp", minerAddr[2]+":"+minerAddr[4], time.Second*2)
	if err != nil {
		return "", "", err
	}

	defer func() {
		_ = conn.Close()
	}()
	addr = minerAddr[2] + ":" + minerAddr[4]
	token = strings.Split(env, ":")[0]
	return addr, token, nil
}
