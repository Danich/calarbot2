package common

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/option"
)

// CreateFirestoreClient создает Firestore клиент с общей конфигурацией
func CreateFirestoreClient(ctx context.Context, config FirestoreConfig) (*firestore.Client, error) {
	var client *firestore.Client
	var err error

	if config.CredPath != "" {
		client, err = firestore.NewClient(ctx, config.ProjectID, option.WithCredentialsFile(config.CredPath))
	} else {
		// Используем Application Default Credentials
		client, err = firestore.NewClient(ctx, config.ProjectID)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Firestore client: %v", err)
	}

	return client, nil
}

// GetModuleCollection возвращает коллекцию для конкретного модуля
// Создает структуру: Calarbot/{moduleName}/data
func GetModuleCollection(client *firestore.Client, moduleName string) *firestore.CollectionRef {
	return client.Collection("Calarbot").Doc(moduleName).Collection("data")
}
