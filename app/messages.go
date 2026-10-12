package app

import (
	"time"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/store"
)

// MessageService 是消息的用例层：读写存储，并在变更后推一帧给订阅者。
type MessageService struct {
	store   *store.MessageStore
	monitor *Monitor
}

func NewMessageService(messages *store.MessageStore, monitor *Monitor) *MessageService {
	return &MessageService{store: messages, monitor: monitor}
}

func (s *MessageService) Store() *store.MessageStore { return s.store }

// Payload 给前端一屏消息：口令消息没解锁就只有 id / 时间，一个字都不多给。
func (s *MessageService) Payload() (core.MessagesPayload, error) {
	messages, err := s.store.List()
	if err != nil {
		return core.MessagesPayload{}, err
	}
	views := make([]any, 0, len(messages))
	locked := 0
	for index := range messages {
		message := &messages[index]
		if message.Kind == core.KindKey {
			views = append(views, message.LockedView())
			locked++
			continue
		}
		views = append(views, message.View())
	}
	return core.MessagesPayload{
		Messages: views,
		Summary:  core.MessageSummary{Total: len(messages), Locked: locked},
	}, nil
}

func (s *MessageService) Create(kind core.Kind, text, key string, refs []store.AttachmentRef) (*core.Message, error) {
	message, err := s.store.Create(kind, text, key, refs, time.Now())
	if err != nil {
		return nil, err
	}
	return message, s.publish()
}

func (s *MessageService) Update(id string, text *string, refs []store.AttachmentRef, replaceAttachments bool, key string) (*core.Message, error) {
	message, err := s.store.Update(id, text, refs, replaceAttachments, key, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	return message, s.publish()
}

func (s *MessageService) Delete(id, key string) ([]string, error) {
	removed, err := s.store.Delete(id, key)
	if err != nil {
		return nil, err
	}
	return removed, s.publish()
}

func (s *MessageService) Get(id, key string) (*core.Message, error) { return s.store.Get(id, key) }

func (s *MessageService) Stats() (store.MessageStats, error) { return s.store.Stats() }

func (s *MessageService) publish() error {
	payload, err := s.Payload()
	if err != nil {
		return err
	}
	s.monitor.Broadcast("messages", RawPayload{Value: payload})
	return nil
}
