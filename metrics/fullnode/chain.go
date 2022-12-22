package fullnode

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xsw1058/lotus-exporter/metrics"
)

type ChainMetric struct {
	Version        string
	ChainHeight    float64
	ChainBaseFee   float64
	ChainTimestamp float64
	NetworkVersion float64
}

// UpdateInfo lotus api 查询逻辑
func (l *FullNode) UpdateInfo() (*ChainMetric, error) {
	l.chain.Lock()
	defer l.chain.Unlock()

	// ChainHead 是每次链状态更新都要随着更新的.
	chainHead, err := l.api.ChainHead(l.ctx)
	if err != nil {
		return nil, err
	}
	l.chain.chainHead = chainHead
	l.chain.baseEpoch = float64(chainHead.Blocks()[0].Height)
	l.chain.baseTimestamp = float64(chainHead.Blocks()[0].Timestamp)

	// StateNetworkVersion 由于可能存在网络版本升级,所以要随着链状态的更新而更新的.
	networkVersion, err := l.api.StateNetworkVersion(l.ctx, l.chain.chainHead.Key())
	if err != nil {
		return nil, err
	}

	if l.chain.networkVersion != networkVersion {
		l.chain.networkVersion = networkVersion
		// StateActorCodeCIDs 是随着网络版本的更新而更新的.
		ciDs, err := l.api.StateActorCodeCIDs(l.ctx, networkVersion)
		if err != nil {
			return nil, err
		}
		l.chain.ciDs = ciDs
	}

	lotusAPIVersion, err := l.api.Version(l.ctx)
	if err != nil {
		return nil, err
	}

	var i ChainMetric
	i.ChainTimestamp = float64(chainHead.Blocks()[0].Timestamp)
	i.ChainBaseFee = float64(chainHead.Blocks()[0].ParentBaseFee.Int64())
	i.ChainHeight = float64(chainHead.Height())
	i.Version = lotusAPIVersion.String()
	i.NetworkVersion = float64(networkVersion)
	return &i, nil
}

type Info struct {
	version     *prometheus.Desc
	chainHeight *prometheus.Desc
	baseFee     *prometheus.Desc
	localTime   *prometheus.Desc
	native      *ChainMetric
}

func NewInfoCollector(data *ChainMetric) (metrics.LotusCollector, error) {

	return &Info{
		version: prometheus.NewDesc(
			prometheus.BuildFQName(NameSpace, "", "info"),
			"lotus daemon information like address version, value is set to network version number",
			[]string{"version"}, nil),
		chainHeight: prometheus.NewDesc(
			prometheus.BuildFQName(NameSpace, "chain", "height"),
			"chain height",
			nil, nil),
		baseFee: prometheus.NewDesc(
			prometheus.BuildFQName(NameSpace, "chain", "basefee"),
			"chain basefee",
			nil, nil),
		localTime: prometheus.NewDesc(
			prometheus.BuildFQName(NameSpace, "chain", "timestamp"),
			"chain time",
			nil, nil),
		native: data,
	}, nil

}
func (i *Info) Update(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(i.version, prometheus.GaugeValue, i.native.NetworkVersion, i.native.Version)
	ch <- prometheus.MustNewConstMetric(i.baseFee, prometheus.GaugeValue, i.native.ChainBaseFee)
	ch <- prometheus.MustNewConstMetric(i.chainHeight, prometheus.CounterValue, i.native.ChainHeight)
	ch <- prometheus.MustNewConstMetric(i.localTime, prometheus.GaugeValue, i.native.ChainTimestamp)
}
