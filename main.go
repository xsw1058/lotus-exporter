package main

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/xsw1058/lotus-exporter/collector"
	"github.com/xsw1058/lotus-exporter/config"
	"github.com/xsw1058/lotus-exporter/metrics"
	"github.com/xsw1058/lotus-exporter/metrics/fullnode"
	"log"
	"net/http"
)

func main() {
	log.SetFlags(23)

	hub := metrics.NewHub()
	opt := config.DefaultOpt()
	log.Printf("conf: %v", opt)
	fullNode := fullnode.MustNewFullNode(context.Background(), opt, hub)
	fullNode.RegisterCollectors()

	hub.Run()

	// 构建http server
	server := http.Server{
		Addr:    opt.ListenPort,
		Handler: http.DefaultServeMux,
	}
	http.Handle("/metrics", newHandler(hub))
	http.Handle("/go", promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>Lotus Exporter</title></head>
			<body>
			<h1>Lotus Exporter By XUSW </h1>
			<p><a href="/metrics">Metrics</a></p>
			</body>
			</html>`))
	})
	log.Println("init done. start http server.....")
	server.ListenAndServe()
}

type handler struct {
	updater metrics.AggregationUpdater
}

func newHandler(u metrics.AggregationUpdater) *handler {
	return &handler{updater: u}
}
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ih, err := h.innerHandler()
	if err != nil {
		http.Error(w, "CCC"+err.Error(), http.StatusInternalServerError)
		return
	}
	ih.ServeHTTP(w, r)
}

func (h *handler) innerHandler() (http.Handler, error) {

	c := collector.NewAllCollector(h.updater)
	promhttp.Handler()
	r := prometheus.NewRegistry()
	err := r.Register(c)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	r.MustRegister(
	// prometheus 自带的一些监控.
	//collectors.NewBuildInfoCollector(),
	//collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	//collectors.NewGoCollector(),
	)
	hh := promhttp.HandlerFor(
		prometheus.Gatherers{r},
		promhttp.HandlerOpts{},
	)

	return hh, nil
}
