package main

import (
	"calarbot2/botModules"
)

type ModuleClientInterface interface {
	Order() int
	IsCalled(payload *botModules.Payload) (bool, error)
	Answer(payload *botModules.Payload) (botModules.RichAnswer, error)
}

type MockModuleClient struct {
	BaseURL         string
	OrderValue      int
	IsCalledResult  bool
	IsCalledError   error
	AnswerResult    botModules.RichAnswer
	AnswerError     error
	IsCalledPayload *botModules.Payload
	AnswerPayload   *botModules.Payload
}

func NewMockModuleClient() *MockModuleClient {
	return &MockModuleClient{
		BaseURL: "http://localhost:8080",
	}
}

func (m *MockModuleClient) Order() int {
	return m.OrderValue
}

func (m *MockModuleClient) IsCalled(payload *botModules.Payload) (bool, error) {
	m.IsCalledPayload = payload
	return m.IsCalledResult, m.IsCalledError
}

func (m *MockModuleClient) Answer(payload *botModules.Payload) (botModules.RichAnswer, error) {
	m.AnswerPayload = payload
	return m.AnswerResult, m.AnswerError
}
