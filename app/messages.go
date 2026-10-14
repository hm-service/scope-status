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

// Payload 给前端一屏消息：每条都给完整内容。
func (s *MessageService) Payload() (core.MessagesPayload, error) {
	messages, err := s.store.List()
	if err != nil {
		return core.MessagesPayload{}, err
	}
	views := make([]core.MessageView, 0, len(messages))
	for index := range messages {
		views = append(views, messages[index].View())
	}
	return core.MessagesPayload{
		Messages: views,
		Summary:  core.MessageSummary{Total: len(messages)},
	}, nil
}

func (s *MessageService) Create(text string, refs []store.AttachmentRef) (*core.Message, error) {
	message, err := s.store.Create(text, refs, time.Now())
	if err != nil {
		return nil, err
	}
	return message, s.publish()
}

func (s *MessageService) Update(id string, text *string, refs []store.AttachmentRef, replaceAttachments bool) (*core.Message, error) {
	message, err := s.store.Update(id, text, refs, replaceAttachments, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	return message, s.publish()
}

func (s *MessageService) Delete(id string) ([]string, error) {
	removed, err := s.store.Delete(id)
	if err != nil {
		return nil, err
	}
	return removed, s.publish()
}

func (s *MessageService) Get(id string) (*core.Message, error) { return s.store.Get(id) }

func (s *MessageService) Stats() (store.MessageStats, error) { return s.store.Stats() }

func (s *MessageService) publish() error {
	payload, err := s.Payload()
	if err != nil {
		return err
	}
	s.monitor.Broadcast("messages", RawPayload{Value: payload})
	return nil
}
