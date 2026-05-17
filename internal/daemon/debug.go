package daemon

import (
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"time"
)

var debugEnabled = os.Getenv("AGENTFLOWD_DEBUG") == "1"

func registerDebugRoutes(mux *http.ServeMux) {
	if !debugEnabled {
		return
	}
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.HandleFunc("/debug/vars", handleDebugVars)
}

type debugVars struct {
	Timestamp   time.Time      `json:"timestamp"`
	MemStats    runtime.MemStats `json:"mem_stats"`
	NumGoroutine int           `json:"num_goroutine"`
	NumCPU      int           `json:"num_cpu"`
	GoVersion   string         `json:"go_version"`
}

func handleDebugVars(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	vars := debugVars{
		Timestamp:    time.Now(),
		MemStats:     ms,
		NumGoroutine: runtime.NumGoroutine(),
		NumCPU:       runtime.NumCPU(),
		GoVersion:    runtime.Version(),
	}
	writeJSON(w, http.StatusOK, vars)
}

func captureMemStats() (allocMB uint64, totalAllocMB uint64, heapMB uint64, sysMB uint64) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Alloc / 1024 / 1024,
		ms.TotalAlloc / 1024 / 1024,
		ms.HeapAlloc / 1024 / 1024,
		ms.Sys / 1024 / 1024
}

func logMemStats(logger *slog.Logger, msg string, attrs ...slog.Attr) {
	if logger == nil {
		return
	}
	alloc, total, heap, sys := captureMemStats()
	allAttrs := append([]slog.Attr{
		slog.Uint64("alloc_mb", alloc),
		slog.Uint64("total_alloc_mb", total),
		slog.Uint64("heap_mb", heap),
		slog.Uint64("sys_mb", sys),
		slog.Int("goroutines", runtime.NumGoroutine()),
	}, attrs...)
	logger.LogAttrs(nil, slog.LevelDebug, msg, allAttrs...)
}
