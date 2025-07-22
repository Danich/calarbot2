package main

import (
	"calarbot2/botModules"
)

// Create a new type that implements the same interface as botModules.ModuleClient
type ModuleClientInterface interface {
	Order() int
	IsCalled(payload *botModules.Payload) (bool, error)
	Answer(payload *botModules.Payload) (string, error)
}

// MockModuleClient is a mock implementation of ModuleClientInterface for testing
type MockModuleClient struct {
	BaseURL         string
	OrderValue      int
	IsCalledResult  bool
	IsCalledError   error
	AnswerResult    string
	AnswerError     error
	IsCalledPayload *botModules.Payload
	AnswerPayload   *botModules.Payload
}

// NewMockModuleClient creates a new MockModuleClient
func NewMockModuleClient() *MockModuleClient {
	return &MockModuleClient{
		BaseURL: "http://localhost:8080",
	}
}

// Order returns the predefined order value
func (m *MockModuleClient) Order() int {
	return m.OrderValue
}

// IsCalled captures the payload and returns the predefined result
func (m *MockModuleClient) IsCalled(payload *botModules.Payload) (bool, error) {
	m.IsCalledPayload = payload
	return m.IsCalledResult, m.IsCalledError
}

// Answer captures the payload and returns the predefined result
func (m *MockModuleClient) Answer(payload *botModules.Payload) (string, error) {
	m.AnswerPayload = payload
	return m.AnswerResult, m.AnswerError
}
