package demo

import (
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xsw1058/lotus-exporter/metrics"
	"math/rand"
	"time"
)

type Demo struct {
}

func NewDemo() *Demo {
	return &Demo{}
}

func (d *Demo) CreateCollector(handler string) (metrics.LotusCollector, error) {
	switch handler {
	case metrics.Demo1Key:
		d1Metric, err := d.UpdateDemo1()
		if err != nil {
			return nil, err
		}
		return NewDemo1Collector(d1Metric)
	case metrics.Demo2Key:
		d2Metric, err := d.UpdateDemo2()
		if err != nil {
			return nil, err
		}
		return NewDemo2Collector(d2Metric)
	case metrics.Demo3Key:
		d3Metric, err := d.UpdateDemo3()
		if err != nil {
			return nil, err
		}
		return NewDemo3Collector(d3Metric)
	}
	return nil, errors.New(fmt.Sprintf("not found handler %v", handler))
}

// ##############################################

// CollectorDemo1 用于描述prometheus的相关数据.
type CollectorDemo1 struct {
	desc        *prometheus.Desc
	labelValues []string
	native      D1Metric
}

// Update 用于prometheus执行Collect方法时进行调用
func (i *CollectorDemo1) Update(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(i.desc, prometheus.GaugeValue, float64(i.native.Value), i.labelValues...)
}

// NewDemo1Collector 构建一个Collector实例
func NewDemo1Collector(data *D1Metric) (metrics.LotusCollector, error) {
	var labelNames, labelValues []string

	for n, v := range data.Labels {
		labelNames = append(labelNames, n)
		labelValues = append(labelValues, v)
	}

	return &CollectorDemo1{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(metrics.NameSpace, "", "demo1"),
			"test demo1",
			labelNames, nil),
		labelValues: labelValues,
		native:      *data,
	}, nil
}

// D1Metric 更新方法返回的数据
type D1Metric struct {
	Labels map[string]string
	Value  int64
}

// UpdateDemo1 调用API执行方法,拿到结果
func (d *Demo) UpdateDemo1() (*D1Metric, error) {
	time.Sleep(time.Millisecond * 500)
	var l = make(map[string]string)
	maxLabel := rand.Intn(2)
	for i := 0; i < maxLabel; i++ {
		labelNames := fmt.Sprintf("label_%v", i)
		labelValues := fmt.Sprintf("lvalue_%v", i)
		l[labelNames] = labelValues
	}
	n := rand.Int63n(9999)
	if (n % 4) == 0 {
		return nil, errors.New("n%x==0")
	}

	return &D1Metric{
		Labels: l,
		Value:  n,
	}, nil
}

// ##############################################

type CollectorDemo2 struct {
	desc        *prometheus.Desc
	labelValues []string
	native      D2Metric
}

// Update 用于prometheus执行Collect方法时进行调用
func (i *CollectorDemo2) Update(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(i.desc, prometheus.GaugeValue, float64(i.native.Value), i.labelValues...)
}

func NewDemo2Collector(data *D2Metric) (metrics.LotusCollector, error) {
	var labelNames, labelValues []string

	for n, v := range data.Labels {
		labelNames = append(labelNames, n)
		labelValues = append(labelValues, v)
	}

	return &CollectorDemo2{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(metrics.NameSpace, "", "demo2"),
			"test demo2",
			labelNames, nil),
		labelValues: labelValues,
		native:      *data,
	}, nil
}

type D2Metric struct {
	Labels map[string]string
	Value  int64
}

func (d *Demo) UpdateDemo2() (*D2Metric, error) {
	time.Sleep(time.Millisecond * 500)
	var l = make(map[string]string)
	maxLabel := rand.Intn(3)
	if maxLabel == 0 {
		maxLabel = 1
	}
	for i := 0; i < maxLabel; i++ {
		labelNames := fmt.Sprintf("label_%v", i)
		labelValues := fmt.Sprintf("lvalue_%v", i)
		l[labelNames] = labelValues
	}
	n := rand.Int63n(9999)
	//if (n % 4) == 0 {
	//	return nil, errors.New("n%x==0")
	//}

	return &D2Metric{
		Labels: l,
		Value:  n,
	}, nil
}

// ##############################################

// ##############################################

type CollectorDemo3 struct {
	desc        *prometheus.Desc
	labelValues []string
	native      D3Metric
}

// Update 用于prometheus执行Collect方法时进行调用
func (i *CollectorDemo3) Update(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(i.desc, prometheus.GaugeValue, float64(i.native.Value), i.labelValues...)
}

func NewDemo3Collector(data *D3Metric) (metrics.LotusCollector, error) {
	var labelNames, labelValues []string

	for n, v := range data.Labels {
		labelNames = append(labelNames, n)
		labelValues = append(labelValues, v)
	}

	return &CollectorDemo3{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(metrics.NameSpace, "", "demo3"),
			"test demo2",
			labelNames, nil),
		labelValues: labelValues,
		native:      *data,
	}, nil
}

type D3Metric struct {
	Labels map[string]string
	Value  int64
}

func (d *Demo) UpdateDemo3() (*D3Metric, error) {
	var l = make(map[string]string)
	maxLabel := rand.Intn(4)
	if maxLabel == 0 {
		maxLabel = 1
	}
	for i := 0; i < maxLabel; i++ {
		labelNames := fmt.Sprintf("label_%v", i)
		labelValues := fmt.Sprintf("lvalue_%v", i)
		l[labelNames] = labelValues
	}
	n := rand.Int63n(9999)
	//if (n % 4) == 0 {
	//	return nil, errors.New("n%x==0")
	//}

	return &D3Metric{
		Labels: l,
		Value:  n,
	}, nil
}

// ##############################################
