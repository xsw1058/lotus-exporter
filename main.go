package main

import (
	"log"
	"sync"
	"time"
)

type StorageAndLoader interface {
	Store(key string, value any)
	Load(key string) any
}

type Storage interface {
	Store(key string, value any)
}

type Loader interface {
	Load(key string) any
}

type MetricsHandler interface {
	Update(storage Storage) error
}

type Mem struct {
	sync.Mutex
	current map[string]any
	new     map[string]any
}

func NewMem() *Mem {
	c := make(map[string]any)
	n := make(map[string]any)
	return &Mem{
		Mutex:   sync.Mutex{},
		current: c,
		new:     n,
	}
}

func (m *Mem) Load(key string) any {
	m.Lock()
	defer m.Unlock()
	v, ok := m.current[key]
	if ok {
		return v
	}
	return nil
}

func (m *Mem) Store(key string, value any) {
	m.Lock()
	defer m.Unlock()
	m.current[key] = value
}

// UpdateRequest 请求更新时发送的数据结构
type UpdateRequest struct {
	// Handlers 更新哪些内容
	Handlers []string
	// Recall 将更新结果发回这个通道
	ReceiveCh chan<- map[string]bool
}

// Hub 向外暴露 UpdateChan, 外界向这个chan发送 UpdateRequest, 实现一次更新.
type Hub struct {
	sync.Mutex
	// UpdateChan 内部从这个chan读 UpdateRequest, 外界向这个chan发送 UpdateRequest.
	UpdateChan chan UpdateRequest

	// 内部收到 UpdateRequest 时,调用相应的函数.
	updateHandlers map[string]MetricsHandler

	// 数据存储, 同时实现读和写接口
	mem StorageAndLoader
}

func NewHub() *Hub {
	return &Hub{
		UpdateChan: make(chan UpdateRequest),
		mem:        NewMem(),
	}
}

// GetUpdateChan 向外暴露只接收 UpdateRequest 的 UpdateChan
func (b *Hub) GetUpdateChan() chan<- UpdateRequest {
	return b.UpdateChan
}

func (b *Hub) RegisterUpdateHandler(name string, handler MetricsHandler) {
	b.Lock()
	defer b.Unlock()
	b.updateHandlers[name] = handler
}

func (b *Hub) run() {
	for {
		select {
		case uReq := <-b.UpdateChan:
			log.Printf("receive a update request: %v ", uReq.Handlers)
			var res = make(map[string]bool)
			for _, name := range uReq.Handlers {
				if h, ok := b.updateHandlers[name]; ok {
					time.Sleep(time.Millisecond * 600)
					err := h.Update(b.mem)
					if err != nil {
						res[name] = false
					} else {
						res[name] = true
					}
				}
			}

			uReq.ReceiveCh <- res
		}
	}
}

type EachEpoch struct {
	UpdateChan  chan<- UpdateRequest
	ReceiveChan chan map[string]bool
}

func NewEachEpoch(updateChan chan<- UpdateRequest) *EachEpoch {
	return &EachEpoch{
		UpdateChan:  updateChan,
		ReceiveChan: make(chan map[string]bool),
	}
}

func (e *EachEpoch) run() {

	go func() {
		for res := range e.ReceiveChan {
			log.Printf("update res: %v", len(res))
		}
	}()

	ticker := time.NewTicker(time.Second * 10)
	var randStr = []string{"ccc", "xxx", "ddd", "jjj"}
	for {
		select {
		// 每隔30秒发送更新请求.
		case <-ticker.C:
			req := UpdateRequest{
				Handlers:  randStr,
				ReceiveCh: e.ReceiveChan,
			}
			e.UpdateChan <- req
		}
	}
}
func main() {
	log.SetFlags(23)
	log.Println("start.....")
	hub := NewHub()
	go hub.run()
	updateChan := hub.GetUpdateChan()
	eachEpoch := NewEachEpoch(updateChan)
	eachEpoch.run()
}
