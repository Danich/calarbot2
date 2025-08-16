package common

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

type MessageLog struct {
	size        int
	currentSize int
	Head        *MessageLogEntry
}
type MessageLogEntry struct {
	Message *tgbotapi.Message
	Next    *MessageLogEntry
}

func NewMessageLog(size int) *MessageLog {
	return &MessageLog{
		size: size,
		Head: nil,
	}
}

func (ml *MessageLog) AddMessage(msg *tgbotapi.Message) {
	entry := &MessageLogEntry{
		Message: msg,
		Next:    nil,
	}

	if ml.Head == nil {
		ml.Head = entry
	} else {
		current := ml.Head
		for current.Next != nil {
			current = current.Next
		}
		current.Next = entry
	}

	if ml.currentSize < ml.size {
		ml.currentSize++
	} else {
		current := ml.Head
		if current != nil {
			ml.Head = current.Next
			current.Next = nil
		}
	}
}

func (ml *MessageLog) GetMessages() []*tgbotapi.Message {
	messages := make([]*tgbotapi.Message, 0, ml.currentSize)
	current := ml.Head
	for current != nil {
		messages = append(messages, current.Message)
		current = current.Next
	}
	return messages
}
