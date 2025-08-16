package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"calarbot2/botModules"
	"calarbot2/common"
)

const ConfigFile = "/cookingAdviceConfig.yaml"

type Module struct {
	order  int
	config Config
	db     DatabaseInterface
}

type Config struct {
	OpenAIToken   string         `yaml:"openai_token"`
	OpenAIBaseURL string         `yaml:"openai_base_url"`
	ModelName     string         `yaml:"model_name"`
	Database      DatabaseConfig `yaml:"database"`
}

type APIRequest struct {
	Photo         string   `json:"photo,omitempty"`
	Ingredients   []string `json:"ingredients"`
	LatestRecipes []string `json:"latest_recipes"`
	Allergies     []string `json:"allergies"`
}

type RecipeSuggestion struct {
	Text             string   `json:"text"`
	IngredientsToBuy []string `json:"ingredients_to_buy"`
}

type APIResponse struct {
	FoodItems         []string                    `json:"food_items"`
	RecipeSuggestions map[string]RecipeSuggestion `json:"recipes_suggestions"`
	Error             string                      `json:"error"`
}

func (m *Module) Order() int {
	return m.order
}

func (m *Module) IsCalled(msg *tgbotapi.Message) bool {
	if msg == nil || msg.Chat == nil || !msg.Chat.IsPrivate() {
		return false
	}

	// Проверяем команды
	if msg.IsCommand() {
		command := msg.Command()
		switch command {
		case "what_to_eat", "что_поесть":
			return true
		case "what_to_make", "что_приготовить":
			return true
		case "recipe", "рецепт":
			return true
		case "made", "приготовил":
			return true
		case "allergy", "аллергия":
			return true
		}
	}

	// Проверяем фото
	if len(msg.Photo) > 0 {
		return true
	}

	return false
}

func (m *Module) Answer(payload *botModules.Payload) (string, error) {
	msg := payload.Msg
	userID := msg.From.ID

	// Инициализируем пользователя в базе данных
	if err := m.initUserIfNotExists(userID); err != nil {
		return "", fmt.Errorf("ошибка инициализации пользователя: %v", err)
	}

	// Обрабатываем команды
	if msg.IsCommand() {
		command := msg.Command()
		args := msg.CommandArguments()

		switch command {
		case "made", "приготовил":
			if args == "" {
				return "Пожалуйста, укажите название рецепта после команды /made\nПример: /made борщ", nil
			}
			return m.addRecipe(userID, args)

		case "allergy", "аллергия":
			if args == "" {
				return "Пожалуйста, укажите ингредиент после команды /allergy\nП��имер: /allergy орехи", nil
			}
			return m.addAllergy(userID, args)

		case "what_to_eat", "что_поесть", "what_to_make", "что_приготовить", "recipe", "рецепт":
			return m.getRecommendations(userID, msg)
		}
	}

	// Обрабатываем фото
	if len(msg.Photo) > 0 {
		return m.getRecommendations(userID, msg)
	}

	return "Неизвестная команда", nil
}

func (m *Module) initUserIfNotExists(userID int64) error {
	ctx := context.Background()
	exists, err := m.db.UserExists(ctx, userID)
	if err != nil {
		return err
	}

	if !exists {
		return m.db.CreateUser(ctx, userID)
	}
	return nil
}

func (m *Module) addRecipe(userID int64, recipe string) (string, error) {
	ctx := context.Background()
	err := m.db.AddRecipe(ctx, userID, recipe)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Рецепт '%s' добавлен в ваш список приготовленных блюд!", recipe), nil
}

func (m *Module) addAllergy(userID int64, ingredient string) (string, error) {
	ctx := context.Background()
	err := m.db.AddAllergy(ctx, userID, ingredient)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Ингредиент '%s' добавлен в список ваших аллергий", ingredient), nil
}

func (m *Module) getRecommendations(userID int64, msg *tgbotapi.Message) (string, error) {
	// Получаем данн��е пользователя
	ingredients, err := m.getUserIngredients(userID)
	if err != nil {
		return "", err
	}

	recipes, err := m.getUserRecipes(userID)
	if err != nil {
		return "", err
	}

	allergies, err := m.getUserAllergies(userID)
	if err != nil {
		return "", err
	}

	// Подготавливаем запрос к API
	apiReq := APIRequest{
		Ingredients:   ingredients,
		LatestRecipes: recipes,
		Allergies:     allergies,
	}

	// Если есть фото, кодируем его в base64
	if len(msg.Photo) > 0 {
		// Берем фото наилучшего качества
		photo := msg.Photo[len(msg.Photo)-1]
		photoData, err := m.getPhotoData(photo.FileID)
		if err != nil {
			return "", fmt.Errorf("ошибка получения фото: %v", err)
		}
		apiReq.Photo = base64.StdEncoding.EncodeToString(photoData)
	}

	// Отправляем запрос к OpenAI
	response, err := m.callOpenAI(apiReq)
	if err != nil {
		return "", err
	}

	// Обрабатываем ответ
	if response.Error != "" {
		return response.Error, nil
	}

	// Сохраняем найденные продукты
	if len(response.FoodItems) > 0 {
		err = m.updateUserIngredients(userID, response.FoodItems)
		if err != nil {
			return "", err
		}
	}

	// Формируем ответ пользователю
	return m.formatRecipes(response.RecipeSuggestions), nil
}

func (m *Module) getUserIngredients(userID int64) ([]string, error) {
	ctx := context.Background()
	return m.db.GetUserIngredients(ctx, userID)
}

func (m *Module) getUserRecipes(userID int64) ([]string, error) {
	ctx := context.Background()
	return m.db.GetUserRecipes(ctx, userID)
}

func (m *Module) getUserAllergies(userID int64) ([]string, error) {
	ctx := context.Background()
	return m.db.GetUserAllergies(ctx, userID)
}

func (m *Module) updateUserIngredients(userID int64, ingredients []string) error {
	ctx := context.Background()
	return m.db.SetUserIngredients(ctx, userID, ingredients)
}

func (m *Module) getPhotoData(fileID string) ([]byte, error) {
	// Здесь должна быть логика получения фото через Telegram Bot API
	// Для простоты возвращаем пустой массив байтов
	// В реальной реализации нужно использовать bot.GetFile() и скачать файл
	return []byte{}, nil
}

func (m *Module) callOpenAI(request APIRequest) (*APIResponse, error) {
	client := openai.NewClient(
		option.WithAPIKey(m.config.OpenAIToken),
		option.WithBaseURL(m.config.OpenAIBaseURL),
	)

	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	userMessage := fmt.Sprintf(`Данные пользователя: %s

Ответь в форм��те JSON:
{
  "food_items": ["список найденных продуктов"],
  "recipes_suggestions": {
    "название_рецепта_1": {
      "text": "текст рецепта", 
      "ingredients_to_buy": ["ингредиент1", "ингредиент2"]
    }
  },
  "error": ""
}`, string(requestJSON))

	// Используем Response API - промпт зашит в ChatGPT
	chatCompletion, err := client.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(userMessage),
		},
		Model: m.config.ModelName,
	})
	if err != nil {
		return nil, fmt.Errorf("ошибк�� вызова OpenAI API: %v", err)
	}

	responseText := chatCompletion.Choices[0].Message.Content

	// Парсим JSON ответ
	var response APIResponse
	err = json.Unmarshal([]byte(responseText), &response)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа от OpenAI: %v, ответ: %s", err, responseText)
	}

	return &response, nil
}

func (m *Module) formatRecipes(recipes map[string]RecipeSuggestion) string {
	if len(recipes) == 0 {
		return "К сожалению, не удалось найти подходящие рецепты"
	}

	var result strings.Builder
	for name, recipe := range recipes {
		result.WriteString(fmt.Sprintf("%s\n%s\n", name, recipe.Text))
		if len(recipe.IngredientsToBuy) > 0 {
			result.WriteString(fmt.Sprintf("надо докупить %s\n", strings.Join(recipe.IngredientsToBuy, ", ")))
		}
		result.WriteString("\n")
	}

	return strings.TrimSpace(result.String())
}

func (m *Module) initDB() error {
	ctx := context.Background()

	// Создаем экземпляр базы данных в зависимости от типа
	switch m.config.Database.Type {
	case "firestore":
		m.db = NewFirestoreDatabase(
			m.config.Database.ProjectID,
			m.config.Database.CredPath,
			m.config.Database.Collection,
		)
	case "local":
		fallthrough
	default:
		m.db = NewLocalDatabase(m.config.Database.LocalPath)
	}

	// Подключаемся к базе данных
	return m.db.Connect(ctx)
}

func main() {
	order := 500
	if len(os.Args) > 1 {
		_, _ = fmt.Sscanf(os.Args[1], "%d", &order)
	}

	config := Config{}
	err := common.ReadConfig(ConfigFile, &config)
	if err != nil {
		fmt.Println("Ошибка конфигурации:", err)
		return
	}

	// Устанавливаем значения по умолчанию
	if config.Database.Type == "" {
		config.Database.Type = "local"
	}
	if config.Database.LocalPath == "" {
		config.Database.LocalPath = "./cooking_advice.db"
	}
	if config.Database.Collection == "" {
		config.Database.Collection = "cooking_users"
	}
	if config.ModelName == "" {
		config.ModelName = "gpt-3.5-turbo"
	}

	module := &Module{order: order, config: config}

	if err := module.initDB(); err != nil {
		fmt.Println("Ошибка инициализации базы данных:", err)
		return
	}

	// Закрываем соединение с базой данных при завершении
	defer func() {
		if module.db != nil {
			module.db.Close()
		}
	}()

	if err := botModules.RunModuleServer(module, ":8080", 0); err != nil {
		fmt.Println(err)
	}
}
