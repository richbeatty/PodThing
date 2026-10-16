package player

import (
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"errors"
	"os"
	"sync/atomic"
)

//go:embed web/index.html
var indexHTML string

type Frames struct{ PNG atomic.Value }

func (a *App) Handler(frames *Frames) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src 'self'; connect-src 'self'")
		_, _ = io.WriteString(w, indexHTML)
	})
	mux.HandleFunc("GET /frame.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		b, _ := frames.PNG.Load().([]byte)
		if len(b) == 0 {
			http.Error(w, "frame not ready", 503)
			return
		}
		_, _ = w.Write(b)
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(a.Snapshot())
	})
	mux.HandleFunc("GET /api/report", func(w http.ResponseWriter, r *http.Request) {
		b, e := a.Report()
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="podthing-report.json"`)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	})
	// Recovery downloads the saved *host* session, never a new snapshot
	// generated after changing the USB role.
	mux.HandleFunc("GET /api/report/saved",func(w http.ResponseWriter,r *http.Request){
		b,e:=a.SavedSessionReport()
		if e!=nil {
			if errors.Is(e,os.ErrNotExist) {
				http.Error(w,"No saved session. Save report while in host mode first.",404)
				return
			}
			http.Error(w,e.Error(),500)
			return
		}
		w.Header().Set("Content-Type","application/json")
		w.Header().Set("Content-Disposition",`attachment; filename="podthing-last-session.json"`)
		w.Header().Set("Cache-Control","no-store")
		_,_=w.Write(b)
	})
	mux.HandleFunc("POST /api/action", func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != r.Host {
				http.Error(w, "cross-origin action rejected", 403)
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var v Action
		if e := decoder.Decode(&v); e != nil {
			http.Error(w, "invalid action", 400)
			return
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			http.Error(w, "unexpected data", 400)
			return
		}
		e := a.Action(v)
		w.Header().Set("Content-Type", "application/json")
		if e != nil {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": e.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		mux.ServeHTTP(w, r)
	})
}
