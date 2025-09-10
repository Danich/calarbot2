package common

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/option"
)

// FirestoreClient общий клиент для всего бота
type FirestoreClient struct {
	client    *firestore.Client
	projectID string
}

// FirestoreConfig конфигурация для Firestore
type FirestoreConfig struct {
	ProjectID string `yaml:"project_id"`
	CredPath  string `yaml:"credentials_path"`
}

// NewFirestoreClient создает новый общий Firestore клиент
func NewFirestoreClient(config FirestoreConfig) *FirestoreClient {
	return &FirestoreClient{
		projectID: config.ProjectID,
	}
}

// Connect подключается к Firestore
func (fc *FirestoreClient) Connect(ctx context.Context, config FirestoreConfig) error {
	var client *firestore.Client
	var err error

	if config.CredPath != "" {
		client, err = firestore.NewClient(ctx, config.ProjectID, option.WithCredentialsFile(config.CredPath))
	} else {
		// Используем Application Default Credentials
		client, err = firestore.NewClient(ctx, config.ProjectID)
	}

	if err != nil {
		return fmt.Errorf("failed to create Firestore client: %v", err)
	}

	fc.client = client
	return nil
}

// Close закрывает соединение с Firestore
func (fc *FirestoreClient) Close() error {
	if fc.client != nil {
		return fc.client.Close()
	}
	return nil
}

// GetCollection возвращает ссылку на коллекцию для конкретного модуля
// Создает структуру: Calarbot/{moduleName}/documents
func (fc *FirestoreClient) GetCollection(moduleName string) *firestore.CollectionRef {
	return fc.client.Collection("Calarbot").Doc(moduleName).Collection("data")
}

// GetEngineCollection возвращает коллекцию для Engine
func (fc *FirestoreClient) GetEngineCollection() *firestore.CollectionRef {
	return fc.GetCollection("Engine")
}

// GetModuleCollection возвращает коллекцию для конкретного модуля
func (fc *FirestoreClient) GetModuleCollection(moduleName string) *firestore.CollectionRef {
	return fc.GetCollection(moduleName)
}

// GetClient возвращает прямой доступ к Firestore клиенту (для особых случаев)
func (fc *FirestoreClient) GetClient() *firestore.Client {
	return fc.client
}
