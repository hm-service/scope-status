package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/i18n"
)

const ssePingInterval = 15 * time.Second

type helloView struct {
	StreamInterval float64 `json:"streamInterval"`
}

// GET /api/events：一次连接，之后服务端主动推每次探测结果与消息变更。
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	payload, err := s.opts.App.Messages.Payload()
	if err != nil {
		writeError(w, r, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, r, core.NewError(http.StatusInternalServerError, i18n.KeyHTTPNoFlush))
		return
	}

	monitor := s.opts.App.Monitor
	sub := monitor.Subscribe(langOf(r))
	defer monitor.Unsubscribe(sub)

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if err := writeEvent(w, "messages", payload); err != nil {
		return
	}
	if snapshot := monitor.Current(); snapshot != nil {
		if err := writeEvent(w, "snapshot", SnapshotView(snapshot, sub.Lang(), true)); err != nil {
			return
		}
	} else if err := writeEvent(w, "hello", helloView{StreamInterval: monitor.StreamInterval().Seconds()}); err != nil {
		return
	}
	flusher.Flush()

	ping := time.NewTicker(ssePingInterval)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-sub.Notify():
			for _, event := range sub.Drain() {
				if err := writeEvent(w, event.Name, event.Payload.Render(sub.Lang())); err != nil {
					return
				}
			}
			flusher.Flush()
		}
	}
}

func writeEvent(w io.Writer, name string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, body)
	return err
}
