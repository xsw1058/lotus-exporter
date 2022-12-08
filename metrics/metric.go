package metrics

import (
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"sync"
	"time"
)

type LotusCollector interface {
	Update(ch chan<- prometheus.Metric)
}
type LotusMetricsHandler interface {
	CreateCollector(handlerKey string) (LotusCollector, error)
}

type AggregationUpdater interface {
	Aggregation() <-chan *ScrapeResult
}

type ScrapeResult struct {
	Status  ScrapeStatus
	Builder LotusCollector
}

// ScrapeStatus 更新完毕后返回的结果.
type ScrapeStatus struct {
	HandlerKey string
	// 哪些Handler更新成功或失败
	Success bool
	// 时间戳
	StartAt time.Time
	// 耗时
	Duration time.Duration
}

type Hub struct {
	sync.Mutex

	// 内部收到 UpdateRequest 时,调用相应的函数.
	metricsHandlers map[string]LotusMetricsHandler

	// 数据存储, 同时实现读和写接口
	mem map[string]*ScrapeResult
}

func NewHub() *Hub {
	return &Hub{
		mem:             make(map[string]*ScrapeResult),
		metricsHandlers: make(map[string]LotusMetricsHandler),
		Mutex:           sync.Mutex{},
	}
}

func (b *Hub) RegisterHandler(name string, handler LotusMetricsHandler) {
	b.Lock()
	defer b.Unlock()

	b.metricsHandlers[name] = handler
}

func (b *Hub) Store(key string, res *ScrapeResult) {
	b.Lock()
	defer b.Unlock()
	b.mem[key] = res
}

func (b *Hub) SingleScrape(handlerKey string) error {
	h, ok := b.metricsHandlers[handlerKey]
	if !ok {
		return errors.New(fmt.Sprintf("%v has not regist", handlerKey))
	}

	start := time.Now()
	var success = true
	collector, err := h.CreateCollector(handlerKey)
	if err != nil {
		log.Printf("%v aggregation update: %v ", handlerKey, err)
		success = false
	}

	r := &ScrapeResult{
		Status: ScrapeStatus{
			HandlerKey: handlerKey,
			Success:    success,
			StartAt:    start,
			Duration:   time.Now().Sub(start),
		},
		Builder: collector,
	}
	b.Store(handlerKey, r)
	return nil
}

func (b *Hub) Aggregation() <-chan *ScrapeResult {

	for _, key := range RightNowUpdate {
		// 跳过未注册的.
		err := b.SingleScrape(key)
		if err != nil {
			continue
		}

	}
	ch := make(chan *ScrapeResult, len(b.mem))
	defer close(ch)

	for _, res := range b.mem {
		ch <- res
	}
	return ch
}

func (b *Hub) Run() {
	// 首次启动进行调用更新
	for key := range b.metricsHandlers {
		go func(key string) {
			b.SingleScrape(key)
		}(key)
	}

	tickerEpoch := time.NewTicker(time.Second * 10)
	tickerDeadline := time.NewTicker(time.Second * 30)

	for {
		select {
		// 每隔30秒发送更新请求.
		case <-tickerEpoch.C:

			for _, key := range EachEpochUpdate {
				err := b.SingleScrape(key)
				if err != nil {
					continue
				}
			}

		case <-tickerDeadline.C:

			for _, key := range EachDeadlineUpdate {
				err := b.SingleScrape(key)
				if err != nil {
					continue
				}
			}
		}
	}
}
