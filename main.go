package main

import (
	"flag"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/xsw1058/lotus-exporter/collector"
	"github.com/xsw1058/lotus-exporter/metrics"
	"github.com/xsw1058/lotus-exporter/metrics/demo"
	"log"
	"net/http"
)

func main() {
	log.SetFlags(23)
	log.Println("start.....")
	a := demo.NewDemo()
	hub := metrics.NewHub()
	hub.RegisterHandler(metrics.Demo1Key, a)
	hub.RegisterHandler(metrics.Demo2Key, a)
	hub.RegisterHandler(metrics.Demo3Key, a)

	go hub.Run()

	listen := flag.String("l", ":9002", "listen port")
	flag.Parse()

	// 构建http server
	server := http.Server{
		Addr:    *listen,
		Handler: http.DefaultServeMux,
	}
	http.Handle("/metrics", newHandler(hub))
	http.Handle("/test", promhttp.Handler())
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
