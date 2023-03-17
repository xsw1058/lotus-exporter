package fullnode

import (
	"errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xsw1058/lotus-exporter/metrics"
	"github.com/xsw1058/lotus-exporter/metrics/filfox"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ActorTransferStatus struct {
	ActorBase
	FirstTransfer  filfox.TransferFilFox
	LatestTransfer filfox.TransferFilFox
	TransferCount  int
}

type Transactions struct {
	SendSummary    *prometheus.GaugeVec
	ReceiveSummary *prometheus.GaugeVec
	TransferStatus *prometheus.GaugeVec
	native         map[ActorTransferStatus]filfox.SummaryDetails
}

func NewTransactionsCollector(data map[ActorTransferStatus]filfox.SummaryDetails) (metrics.LotusCollector, error) {
	return &Transactions{
		SendSummary: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "transfer_send",
				Help:      "transfer_send summary",
			}, []string{"actor_id", "tag", "miner_id", "duration", "peer", "peer_tag"}),
		ReceiveSummary: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "transfer_receive",
				Help:      "transfer_receive summary",
			}, []string{"actor_id", "tag", "miner_id", "duration", "peer", "peer_tag"}),
		TransferStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "transfer_status",
				Help:      "transfer_receive summary",
			}, []string{"actor_id", "tag", "miner_id", "start_timestamp", "latest_timestamp"}),
		native: data,
	}, nil
}
func (s *Transactions) Update(ch chan<- prometheus.Metric) {
	s.SendSummary.Reset()
	s.ReceiveSummary.Reset()
	s.TransferStatus.Reset()
	for actorTransferStatus, summary := range s.native {
		s.TransferStatus.WithLabelValues(actorTransferStatus.ActorID.String(), actorTransferStatus.Tag, actorTransferStatus.MinerID.String(), strconv.Itoa(int(actorTransferStatus.FirstTransfer.Timestamp)), strconv.Itoa(int(actorTransferStatus.LatestTransfer.Timestamp))).Set(float64(actorTransferStatus.TransferCount))
		for _, detail := range summary.Details {
			for peer, value := range detail.Transfer.Send {
				s.SendSummary.WithLabelValues(actorTransferStatus.ActorID.String(), actorTransferStatus.Tag, actorTransferStatus.MinerID.String(), detail.String(), peer.Address, peer.Tag).Set(value)
			}
			for peer, value := range detail.Transfer.Receive {
				s.ReceiveSummary.WithLabelValues(actorTransferStatus.ActorID.String(), actorTransferStatus.Tag, actorTransferStatus.MinerID.String(), detail.String(), peer.Address, peer.Tag).Set(value)
			}
		}
	}
	s.SendSummary.Collect(ch)
	s.ReceiveSummary.Collect(ch)
	s.TransferStatus.Collect(ch)
}

func (l *FullNode) TransactionsSummary() (metrics.LotusCollector, error) {
	filFox := filfox.NewFilFox()

	pendingTransfers := make(map[ActorBase]filfox.Transfers)
	now := time.Now()
	// 从当前时间前推2个月
	earliestStartAt := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -1, 0)
	// 用于从filFox获取需要的最新的交易信息, 并放入到 pendingTransfers 中.
	for _, actor := range l.chain.attentionActor {
		if actor.ActorType == MinerKey {
			continue
		}
		if strings.Contains(actor.Tag, "control") {
			continue
		}
		if strings.Contains(actor.Tag, "worker") {
			continue
		}

		var since time.Time
		if details, ok := l.transferDetail[actor]; ok {
			// 存在历史数据, 只需要获取最新数据即可
			latestPageTransfer, _, err := filFox.GetLatestPageTransfer(actor.ActorID.String())
			if err != nil {
				log.Warnw("get latest page", "tag", actor.Tag, "err", err)
				continue
			}

			// 获取第一页的最后一个时间戳比历史数据还大, 表示还需要获取更多数据.
			if latestPageTransfer[len(latestPageTransfer)-1].Timestamp > details[len(details)-1].Timestamp {
				since = time.Unix(details[len(details)-1].Timestamp, 0)
			}

			pendingTransfers[actor] = append(pendingTransfers[actor], latestPageTransfer...)

		} else {
			since = earliestStartAt
		}

		// 值为空, 表示不需要获取更多数据
		if since.IsZero() {
			continue
		}

		transferSince, _, err := filFox.GetTransferSince(actor.ActorID.String(), since)
		if err != nil {
			log.Warnw("get transfers", "tag", actor.Tag, "since", since.String(), "err", err)
			continue
		}

		pendingTransfers[actor] = append(pendingTransfers[actor], transferSince...)

	}

	actorsSummaries := make(map[ActorTransferStatus]filfox.SummaryDetails)

	for actor, ps := range pendingTransfers {

		var tmpTransfers filfox.Transfers

		if details, ok := l.transferDetail[actor]; ok {
			// 合并已有数据
			tmpTransfers = append(tmpTransfers, details...)
		}

		tmpTransfers = append(tmpTransfers, ps...)
		// 去重
		allTransfers := tmpTransfers.RemoveDuplicate()
		// 排序
		sort.Sort(allTransfers)

		// 将已处理后的transfer保存到实例中存储.
		l.transferDetail[actor] = allTransfers

		var summaries = filfox.NewSummaryDetails()

		var transferStatus = ActorTransferStatus{
			ActorBase:      actor,
			FirstTransfer:  allTransfers[0],
			LatestTransfer: allTransfers[len(allTransfers)-1],
			TransferCount:  len(allTransfers),
		}

		for _, transfer := range allTransfers {
			at := time.Unix(transfer.Timestamp, 0)
			nanoFilStr := transfer.Value[:len(transfer.Value)-9]
			nanoFil, err := strconv.Atoi(nanoFilStr)
			if err != nil {
				log.Debugw("value convert nano", "err", err, "type", transfer.Type, "timestamp", at.Format("2006-01-02 15:04:05"))
				continue
			}
			var valueFilFloat64 = float64(nanoFil) / 1000000000

			for _, s := range summaries.Details {
				if s.Between(at) {
					switch transfer.Type {
					case filfox.Burn:
					case filfox.Reward:
					case filfox.Send:
						peer := filfox.PeerActor{
							Address: transfer.To,
							Tag:     "",
						}

						s.Transfer.Send[peer] += math.Abs(valueFilFloat64)
					case filfox.Receive:
						peer := filfox.PeerActor{
							Address: transfer.From,
							Tag:     "",
						}
						s.Transfer.Receive[peer] += math.Abs(valueFilFloat64)
					default:
						log.Debugw("not support type", "type", transfer.Type, "timestamp", at.Format("2006-01-02 15:04:05"))
					}
				}
			}
		}
		actorsSummaries[transferStatus] = summaries
	}

	if len(actorsSummaries) == 0 {
		return nil, errors.New("all actor no found transfer")
	}
	return NewTransactionsCollector(actorsSummaries)
}
