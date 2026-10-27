package main

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
)

var durationBuckets = [...]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type metricKey struct {
	method string
	route  string
	status int
}

type sample struct {
	count   uint64
	sum     float64
	buckets [len(durationBuckets)]uint64
}

type metrics struct {
	mu      sync.Mutex
	samples map[metricKey]sample
}

func newMetrics() *metrics {
	return &metrics{samples: make(map[metricKey]sample)}
}

func (m *metrics) observe(method, route string, status int, seconds float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := metricKey{method: method, route: route, status: status}
	value := m.samples[key]
	value.count++
	value.sum += seconds
	for index, bound := range durationBuckets {
		if seconds <= bound {
			value.buckets[index]++
		}
	}
	m.samples[key] = value
}

func (m *metrics) serve(w http.ResponseWriter) {
	m.mu.Lock()
	copyOfSamples := make(map[metricKey]sample, len(m.samples))
	for key, value := range m.samples {
		copyOfSamples[key] = value
	}
	m.mu.Unlock()
	keys := make([]metricKey, 0, len(copyOfSamples))
	for key := range copyOfSamples {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].status < keys[j].status
	})
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintln(w, "# HELP kubelab_http_requests_total HTTP requests handled by this service.")
	fmt.Fprintln(w, "# TYPE kubelab_http_requests_total counter")
	for _, key := range keys {
		fmt.Fprintf(w, "kubelab_http_requests_total%s %d\n", labels(key), copyOfSamples[key].count)
	}
	fmt.Fprintln(w, "# HELP kubelab_http_request_duration_seconds HTTP request duration in seconds.")
	fmt.Fprintln(w, "# TYPE kubelab_http_request_duration_seconds histogram")
	for _, key := range keys {
		value := copyOfSamples[key]
		for index, bound := range durationBuckets {
			fmt.Fprintf(w, "kubelab_http_request_duration_seconds_bucket%s %d\n", labelsWithBucket(key, fmt.Sprintf("%g", bound)), value.buckets[index])
		}
		fmt.Fprintf(w, "kubelab_http_request_duration_seconds_bucket%s %d\n", labelsWithBucket(key, "+Inf"), value.count)
		fmt.Fprintf(w, "kubelab_http_request_duration_seconds_sum%s %g\n", labels(key), value.sum)
		fmt.Fprintf(w, "kubelab_http_request_duration_seconds_count%s %d\n", labels(key), value.count)
	}
}

func labels(key metricKey) string {
	return fmt.Sprintf("{service=\"api\",method=%q,route=%q,status=%q}", key.method, key.route, fmt.Sprintf("%d", key.status))
}

func labelsWithBucket(key metricKey, bound string) string {
	return fmt.Sprintf("{service=\"api\",method=%q,route=%q,status=%q,le=%q}", key.method, key.route, fmt.Sprintf("%d", key.status), bound)
}
