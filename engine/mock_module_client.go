package main

import (
	"calarbot2/botModules"
)

type ModuleClientInterface interface {
	Register() (botModules.Registration, error)
	IsCalled(payload *botModules.Payload) (bool, error)
	Answer(payload *botModules.Payload) (botModules.RichAnswer, error)
}

type MockModuleClient struct {
	BaseURL           string
	RegistrationValue botModules.Registration
	RegistrationError error
	IsCalledResult    bool
	IsCalledError     error
	AnswerResult      botModules.RichAnswer
	AnswerError       error
	IsCalledPayload   *botModules.Payload
	AnswerPayload     *botModules.Payload
}

func NewMockModuleClient() *MockModuleClient {
	return &MockModuleClient{
		BaseURL: "http://localhost:8080",
	}
}

func (m *MockModuleClient) Register() (botModules.Registration, error) {
	return m.RegistrationValue, m.RegistrationError
}

func (m *MockModuleClient) IsCalled(payload *botModules.Payload) (bool, error) {
	m.IsCalledPayload = payload
	return m.IsCalledResult, m.IsCalledError
}

func (m *MockModuleClient) Answer(payload *botModules.Payload) (botModules.RichAnswer, error) {
	m.AnswerPayload = payload
	return m.AnswerResult, m.AnswerError
}
