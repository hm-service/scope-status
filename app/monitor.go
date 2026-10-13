package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/i18n"
	"github.com/hm-service/scope-status/store"
)

const subscriberQueue = 4

// Payload 是能按语言渲染的事件负载：快照要按 Accept-Language 出文案，消息列表不用。
type Payload interface {
	Render(lang i18n.Lang) any
}

type RawPayload struct{ Value any }

func (p RawPayload) Render(i18n.Lang) any { return p.Value }

type Event struct {
	Name    string
	Payload Payload
}

// SnapshotViewFunc 由 HTTP 层提供：快照 -> 可直接序列化的形状。
type SnapshotViewFunc func(snapshot *core.Snapshot, lang i18n.Lang, reused bool) any

type snapshotPayload struct {
	snapshot *core.Snapshot
	reused   bool
	view     SnapshotViewFunc
}

func (p snapshotPayload) Render(lang i18n.Lang) any {
	if p.view == nil {
		panic("app: 快照渲染函数没有接线")
	}
	return p.view(p.snapshot, lang, p.reused)
}

type subscriber struct {
	lang   i18n.Lang
	mu     sync.Mutex
	queue  []Event
	notify chan struct{}
}

func newSubscriber(lang i18n.Lang) *subscriber {
	return &subscriber{lang: lang, notify: make(chan struct{}, 1)}
}

// push 队列满了就丢掉最旧的一帧：订阅者掉队不该拖住服务端。
func (s *subscriber) push(event Event) {
	s.mu.Lock()
	if len(s.queue) >= subscriberQueue {
		s.queue = s.queue[1:]
	}
	s.queue = append(s.queue, event)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *subscriber) Drain() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.queue
	s.queue = nil
	return events
}

func (s *subscriber) Lang() i18n.Lang         { return s.lang }
func (s *subscriber) Notify() <-chan struct{} { return s.notify }

type flight struct {
	done     chan struct{}
	snapshot *core.Snapshot
	err      error
}

// Monitor 并发探测所有条目，把结果放一份共享快照里；SSE 订阅者共用这一份。
type Monitor struct {
	mu             sync.RWMutex
	source         *core.Source
	sourcePath     string
	streamInterval time.Duration
	snapshot       *core.Snapshot
	reloadCount    int
	lastDurationMs float64
	favicons       *core.FaviconCache
	view           SnapshotViewFunc

	subMu sync.Mutex
	subs  map[*subscriber]struct{}
	wake  chan struct{}

	flightMu sync.Mutex
	flight   *flight
}

func NewMonitor(source *core.Source, sourcePath string, streamInterval time.Duration) *Monitor {
	return &Monitor{
		source:         source,
		sourcePath:     sourcePath,
		streamInterval: max(0, streamInterval),
		favicons:       core.NewFaviconCache(),
		subs:           make(map[*subscriber]struct{}),
		wake:           make(chan struct{}, 1),
	}
}

func (m *Monitor) SetSnapshotView(view SnapshotViewFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.view = view
}

func (m *Monitor) Source() *core.Source {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.source
}

func (m *Monitor) SourcePath() string { return m.sourcePath }

func (m *Monitor) StreamInterval() time.Duration { return m.streamInterval }

func (m *Monitor) ReloadCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.reloadCount
}

func (m *Monitor) LastDurationMs() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastDurationMs
}

func (m *Monitor) Current() *core.Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot
}

func (m *Monitor) Subscribers() int {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	return len(m.subs)
}

// Snapshot 返回共享快照；还没有快照时返回 nil（HTTP 层回 204）。
//
// 有 SSE 循环时快照一直算新，请求恒定时间返回；没有循环（STREAM_INTERVAL=0）时才按需探一次。
func (m *Monitor) Snapshot(force bool) (*core.Snapshot, bool, error) {
	if !force && m.streamInterval > 0 {
		if current := m.Current(); current != nil {
			return current, true, nil
		}
		return nil, false, nil
	}
	snapshot, err := m.Refresh()
	if err != nil {
		return nil, false, err
	}
	return snapshot, false, nil
}

// Refresh 探一次，更新共享快照并推给所有订阅者；同一时刻的并发调用共用一次探测。
func (m *Monitor) Refresh() (*core.Snapshot, error) {
	m.flightMu.Lock()
	if running := m.flight; running != nil {
		m.flightMu.Unlock()
		<-running.done
		return running.snapshot, running.err
	}
	running := &flight{done: make(chan struct{})}
	m.flight = running
	m.flightMu.Unlock()

	snapshot, err := m.collect()

	m.mu.Lock()
	if err == nil {
		m.snapshot = snapshot
		m.lastDurationMs = snapshot.DurationMs
	}
	view := m.view
	m.mu.Unlock()

	running.snapshot, running.err = snapshot, err
	close(running.done)
	m.flightMu.Lock()
	if m.flight == running {
		m.flight = nil
	}
	m.flightMu.Unlock()

	if err != nil {
		return nil, err
	}
	m.Broadcast("snapshot", snapshotPayload{snapshot: snapshot, reused: false, view: view})
	return snapshot, nil
}

// Reload 重读数据目录里的配置：先解析新的，失败就原样保留旧配置。
func (m *Monitor) Reload() (*core.Source, error) {
	source, err := store.LoadSource(m.sourcePath)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.source = source
	m.snapshot = nil
	m.reloadCount++
	m.mu.Unlock()
	return source, nil
}

// Stream 恒定节奏地探测；长期没人订阅就降到每分钟一次。
//
// 探测真的失败（比如网段没了）会记一条日志并结束循环，不吞掉错误假装一切正常。
func (m *Monitor) Stream(ctx context.Context) {
	for {
		interval := m.streamInterval
		if m.Subscribers() == 0 {
			interval = max(interval, time.Minute)
		}
		if m.Current() == nil {
			interval = 0
		}
		if interval > 0 {
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-m.wake:
				// 重新算一遍间隔：现在有人看了，不必再等那一分钟
				timer.Stop()
				continue
			case <-timer.C:
			}
		}
		if _, err := m.Refresh(); err != nil {
			logProbeFailure(err)
			return
		}
	}
}

func logProbeFailure(err error) {
	var business *core.Error
	if errors.As(err, &business) {
		slog.Error("stream loop stopped", "key", string(business.Key), "args", business.Args, "status", business.Status)
		return
	}
	slog.Error("stream loop stopped", "error", err.Error())
}

func (m *Monitor) Broadcast(name string, payload Payload) {
	event := Event{Name: name, Payload: payload}
	m.subMu.Lock()
	targets := make([]*subscriber, 0, len(m.subs))
	for sub := range m.subs {
		targets = append(targets, sub)
	}
	m.subMu.Unlock()
	for _, sub := range targets {
		sub.push(event)
	}
}

func (m *Monitor) Subscribe(lang i18n.Lang) *subscriber {
	sub := newSubscriber(lang)
	m.subMu.Lock()
	m.subs[sub] = struct{}{}
	m.subMu.Unlock()
	// 空闲时循环在睡一分钟，来了订阅者得立刻重估节奏
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return sub
}

func (m *Monitor) Unsubscribe(sub *subscriber) {
	m.subMu.Lock()
	delete(m.subs, sub)
	m.subMu.Unlock()
}

type addressJob struct {
	friend  int
	address string
}

// collect 把所有条目一起丢进有界的并发池：总耗时约等于最慢的那一个探测。
func (m *Monitor) collect() (*core.Snapshot, error) {
	source := m.Source()
	started := time.Now()
	probe := source.Probe

	jobs := make([]addressJob, 0, len(source.Friends))
	for index, friend := range source.Friends {
		for _, address := range friend.Addresses {
			jobs = append(jobs, addressJob{friend: index, address: address})
		}
	}
	addressResults := make([]core.ProbeResult, len(jobs))
	serviceResults := make([]core.ProbeResult, len(source.Services))
	linkResults := make([]core.ProbeResult, len(source.Links))

	semaphore := make(chan struct{}, max(1, probe.Concurrency))
	var wait sync.WaitGroup
	var collectErr error
	var errMu sync.Mutex

	run := func(task func() error) {
		wait.Add(1)
		go func() {
			defer wait.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			if err := task(); err != nil {
				errMu.Lock()
				if collectErr == nil {
					collectErr = err
				}
				errMu.Unlock()
			}
		}()
	}

	for index, job := range jobs {
		run(func() error {
			result, err := core.ProbeAddress(job.address, probe)
			if err != nil {
				return err
			}
			addressResults[index] = result
			return nil
		})
	}
	for index, service := range source.Services {
		run(func() error {
			result, err := core.ProbeService(service, probe.Timeout)
			if err != nil {
				return err
			}
			serviceResults[index] = result
			return nil
		})
	}
	for index, link := range source.Links {
		run(func() error {
			result, err := core.ProbeLink(link, 5*time.Second)
			if err != nil {
				return err
			}
			linkResults[index] = result
			return nil
		})
	}
	wait.Wait()
	if collectErr != nil {
		return nil, collectErr
	}

	byFriend := make([][]core.ProbeResult, len(source.Friends))
	for index, job := range jobs {
		byFriend[job.friend] = append(byFriend[job.friend], addressResults[index])
	}

	friends := make([]core.FriendStatus, len(source.Friends))
	friendsOnline := 0
	for index, friend := range source.Friends {
		status := core.FriendStatus{
			Friend:    friend,
			Addresses: byFriend[index],
			LatencyMs: core.BestLatency(byFriend[index]),
		}
		for _, result := range status.Addresses {
			if result.Online() {
				status.Online = true
				break
			}
		}
		if status.Online {
			friendsOnline++
		}
		friends[index] = status
	}

	services := make([]core.ServiceStatus, len(source.Services))
	servicesOnline := 0
	for index, service := range source.Services {
		result := serviceResults[index]
		status := core.ServiceStatus{
			Service:   service,
			Online:    result.Online(),
			LatencyMs: result.LatencyMs,
			Detail:    result.Detail,
			Status:    result.Status,
		}
		if status.Online {
			servicesOnline++
		}
		services[index] = status
	}

	links := make([]core.LinkStatus, len(source.Links))
	linksOnline := 0
	for index, link := range source.Links {
		result := linkResults[index]
		status := core.LinkStatus{
			Link:       link,
			Online:     result.Online(),
			LatencyMs:  result.LatencyMs,
			Detail:     result.Detail,
			Status:     result.Status,
			FaviconURL: m.favicons.For(link.URL),
		}
		if status.Online {
			linksOnline++
		}
		links[index] = status
	}

	return &core.Snapshot{
		Title:           source.Title,
		TitleKey:        source.TitleKey,
		Method:          probe.Method,
		GeneratedAt:     time.Now(),
		DurationMs:      float64(time.Since(started).Microseconds()) / 1000,
		Reloadable:      m.sourcePath != "",
		ReloadCount:     m.ReloadCount(),
		StreamInterval:  m.streamInterval.Seconds(),
		Summary:         core.NewSummary(len(friends), friendsOnline),
		Groups:          core.GroupFriends(source, friends),
		Friends:         friends,
		Services:        services,
		ServicesGroups:  core.GroupServices(source, services),
		ServicesSummary: core.NewSummary(len(services), servicesOnline),
		Links:           links,
		LinksSummary:    core.NewSummary(len(links), linksOnline),
	}, nil
}
