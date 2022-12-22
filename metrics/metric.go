package metrics

import (
	logging "github.com/ipfs/go-log/v2"
	"github.com/prometheus/client_golang/prometheus"
	"sync"
	"time"
)

var (
	log = logging.Logger("metric")

	rightNowUpdate     []string
	eachEpochUpdate    []string
	eachDeadlineUpdate []string
)

const (
	RightNowScan = iota
	EachEpochScan
	EachDeadlineScan
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

type Register interface {
	RegisterHandler(name string, scanAt int, handler LotusMetricsHandler)
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

func (b *Hub) RegisterHandler(name string, scanAt int, handler LotusMetricsHandler) {
	b.Lock()
	defer b.Unlock()
	switch scanAt {
	case EachEpochScan:
		eachEpochUpdate = append(eachEpochUpdate, name)
	case EachDeadlineScan:
		eachDeadlineUpdate = append(eachDeadlineUpdate, name)
	default:
		rightNowUpdate = append(rightNowUpdate, name)
	}
	b.metricsHandlers[name] = handler
}

func (b *Hub) Store(key string, res *ScrapeResult) {
	b.Lock()
	defer b.Unlock()
	b.mem[key] = res
}

func (b *Hub) AsyncScrape(handlersKey []string) {
	wg := sync.WaitGroup{}

	wg.Add(len(handlersKey))
	for _, handlerKey := range handlersKey {
		go func(k string) {
			defer wg.Done()
			b.SingleScrape(k)
		}(handlerKey)
	}

	wg.Wait()
}
func (b *Hub) SingleScrape(handlerKey string) {
	h, ok := b.metricsHandlers[handlerKey]
	if !ok {
		log.Debugf("%v has not regist", handlerKey)
		return
	}

	start := time.Now()
	var success = true
	collector, err := h.CreateCollector(handlerKey)
	if err != nil {
		log.Warnf("collector %v error: %v ", handlerKey, err)
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

}

func (b *Hub) Aggregation() <-chan *ScrapeResult {
	b.AsyncScrape(rightNowUpdate)
	ch := make(chan *ScrapeResult, len(b.mem))
	defer close(ch)

	for _, res := range b.mem {
		ch <- res
	}
	return ch
}

func (b *Hub) Run() {
	// 首次启动进行调用更新
	wg := sync.WaitGroup{}
	var cs []string
	wg.Add(len(b.metricsHandlers))
	for key := range b.metricsHandlers {
		cs = append(cs, key)
		go func(key string) {
			defer wg.Done()
			defer log.Debugf("%v collector work done", key)
			b.SingleScrape(key)
		}(key)
	}
	log.Infow("register collector result", "len", len(cs), "rightNowUpdate", rightNowUpdate, "eachEpochUpdate", eachEpochUpdate, "eachDeadlineUpdate", eachDeadlineUpdate, "all", cs)
	wg.Wait()
	go func() {
		tickerEpoch := time.NewTicker(time.Second * 30)
		tickerDeadline := time.NewTicker(time.Second * 30 * 60)

		for {
			select {
			// 每隔30秒发送更新请求.
			case <-tickerEpoch.C:
				b.AsyncScrape(eachEpochUpdate)
			case <-tickerDeadline.C:
				b.AsyncScrape(eachDeadlineUpdate)
			}
		}
	}()
}
