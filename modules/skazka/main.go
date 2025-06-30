package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
)

const (
	maxTurns            = 10
	registrationTimeout = 300 * time.Second
	turnTimeout         = 180 * time.Second
	channelID           = -1001742935232
)

// Messages for the game
const (
	msgRegistrationStarted       = "Регистрация запущена. У вас есть пять минут чтобы зарегистрироваться при помощи команды /play"
	msgRegistrationFinished      = "Регистрация завершена. Скоро я приду к вам в личку со следующим ходом. На ход даётся три минуты"
	msgRegistrationFailed        = "Регистрация уже закончилась"
	msgRegistrationOK            = "Записал тебя"
	msgRegistrationAlreadyDone   = "Ты уже записан!"
	msgRegistrationRequiresStart = "Не могу тебя зарегистрировать. Напиши сперва мне в личку /start"
	msgCantPlayAlone             = "Одному играть бессмысленно - никакого праздника."
	msgFirstPlayer               = "Ты начинаешь. Придумай начало для истории. На ход даётся три минуты."
	msgRegularPlayer             = "Тебе выпало продолжить рассказ: "
	msgLastPlayer                = "Закончи рассказ: "
	msgStoryReport               = "Эпичный рассказ от "
	msgTurnTimeout               = "Время вышло, передаю ход дальше"
)

// Lists for anonymous player names
var animals = []string{
	"кот", "пёс", "осел", "козел", "лошадь", "кролик", "трубкозуб", "альбатрос",
	"аллигатор", "удильщик", "муравей", "муравьед", "жерех", "бабуин", "барсук",
	// Add more animals as needed
}

var adjectives = []string{
	"заброшенный", "способный", "абсолютный", "академический", "приемлемый",
	"признанный", "точный", "кислый", "акробатический", "авантюрный",
	// Add more adjectives as needed
}

// GameSession represents a single game session
type GameSession struct {
	chatID          int64
	nowRegistering  bool
	players         map[int64]string // map[userID]username
	text            string
	lastMessage     string
	alive           bool
	waitingForReply int64 // userID of the player we're waiting for
	mu              sync.Mutex
}

// GameStorage manages multiple game sessions
type GameStorage struct {
	sessions map[int64]*GameSession // map[chatID]*GameSession
	mu       sync.Mutex
}

// Module implements the BotModule interface
type Module struct {
	order   int
	storage *GameStorage
	bot     *tgbotapi.BotAPI
}

func (m *Module) Order() int { return m.order }

func (m *Module) IsCalled(msg *tgbotapi.Message) bool {
	if msg.IsCommand() {
		cmd := msg.Command()
		return cmd == "skazka" || cmd == "play"
	}

	// Check if we're waiting for a reply from this user
	m.storage.mu.Lock()
	defer m.storage.mu.Unlock()

	for _, session := range m.storage.sessions {
		session.mu.Lock()
		waiting := session.waitingForReply == msg.From.ID
		session.mu.Unlock()
		if waiting {
			return true
		}
	}

	return false
}

func (m *Module) Answer(payload *botModules.Payload) (string, error) {
	msg := payload.Msg

	if msg.IsCommand() {
		cmd := msg.Command()
		if cmd == "skazka" {
			return m.handleSkazkaCommand(msg)
		} else if cmd == "play" {
			return m.handlePlayCommand(msg)
		}
	}

	// Check if this is a reply to a turn
	m.storage.mu.Lock()
	defer m.storage.mu.Unlock()

	for _, session := range m.storage.sessions {
		session.mu.Lock()
		waiting := session.waitingForReply == msg.From.ID
		session.mu.Unlock()

		if waiting {
			session.mu.Lock()
			session.catchMessage(msg)
			session.mu.Unlock()
			return "", nil // No response needed, the game will continue automatically
		}
	}

	return "Неизвестная команда", nil
}

func (m *Module) handleSkazkaCommand(msg *tgbotapi.Message) (string, error) {
	chatID := msg.Chat.ID

	m.storage.mu.Lock()
	defer m.storage.mu.Unlock()

	// Check if a game is already running in this chat
	if session, exists := m.storage.sessions[chatID]; exists && session.alive {
		return "Игра уже запущена в этом чате", nil
	}

	// Create a new game session
	session := &GameSession{
		chatID:         chatID,
		nowRegistering: true,
		players:        make(map[int64]string),
		alive:          true,
	}
	m.storage.sessions[chatID] = session

	// Start the game in a goroutine
	go m.runGame(session)

	return msgRegistrationStarted, nil
}

func (m *Module) handlePlayCommand(msg *tgbotapi.Message) (string, error) {
	userID := msg.From.ID
	chatID := msg.Chat.ID

	m.storage.mu.Lock()
	defer m.storage.mu.Unlock()

	// Find the game session for this chat
	session, exists := m.storage.sessions[chatID]
	if !exists || !session.alive {
		return "В этом чате нет активной игры", nil
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if !session.nowRegistering {
		return msgRegistrationFailed, nil
	}

	if _, registered := session.players[userID]; registered {
		return msgRegistrationAlreadyDone, nil
	}

	// Try to send a message to the user to check if they've started the bot
	_, err := m.bot.Send(tgbotapi.NewMessage(userID, msgRegistrationOK))
	if err != nil {
		return msgRegistrationRequiresStart, nil
	}

	// Register the player
	session.players[userID] = msg.From.UserName
	return msgRegistrationOK, nil
}

func (m *Module) runGame(session *GameSession) {
	// Registration phase
	m.bot.Send(tgbotapi.NewMessage(session.chatID, msgRegistrationStarted))
	time.Sleep(registrationTimeout)

	session.mu.Lock()
	session.nowRegistering = false
	session.mu.Unlock()

	m.bot.Send(tgbotapi.NewMessage(session.chatID, msgRegistrationFinished))

	// Game phase
	session.mu.Lock()
	playerCount := len(session.players)
	session.mu.Unlock()

	if playerCount < 2 {
		m.bot.Send(tgbotapi.NewMessage(session.chatID, msgCantPlayAlone))

		session.mu.Lock()
		session.alive = false
		session.mu.Unlock()

		m.storage.mu.Lock()
		delete(m.storage.sessions, session.chatID)
		m.storage.mu.Unlock()

		return
	}

	// Create a list of players and shuffle it
	var playerIDs []int64
	session.mu.Lock()
	for id := range session.players {
		playerIDs = append(playerIDs, id)
	}
	session.mu.Unlock()

	rand.Shuffle(len(playerIDs), func(i, j int) {
		playerIDs[i], playerIDs[j] = playerIDs[j], playerIDs[i]
	})

	// Game loop
	counter := 1
	maxTurnsForGame := maxTurns
	if playerCount > maxTurns {
		maxTurnsForGame = playerCount
	}

	for counter <= maxTurnsForGame {
		for _, playerID := range playerIDs {
			if counter > maxTurnsForGame {
				break
			}

			// Prepare message for this player
			var messageToSend string
			session.mu.Lock()
			if session.text == "" {
				messageToSend = msgFirstPlayer
			} else {
				if isLastTurn(counter, playerCount, maxTurnsForGame) {
					messageToSend = msgLastPlayer
				} else {
					messageToSend = msgRegularPlayer
				}

				// Add part of the previous message
				messageToSend = trimLastPart(messageToSend, session.lastMessage)
			}

			// Mark that we're waiting for this player
			session.waitingForReply = playerID
			session.mu.Unlock()

			// Send the message to the player
			m.bot.Send(tgbotapi.NewMessage(playerID, messageToSend))

			// Wait for the player's response or timeout
			timeout := time.After(turnTimeout)
			responded := false

			for !responded {
				select {
				case <-timeout:
					// Player didn't respond in time
					m.bot.Send(tgbotapi.NewMessage(playerID, msgTurnTimeout))
					responded = true
				case <-time.After(1 * time.Second):
					// Check if the player has responded
					session.mu.Lock()
					responded = session.waitingForReply != playerID
					session.mu.Unlock()
				}
			}

			counter++
		}
	}

	// Post the final story
	session.mu.Lock()
	authors := make([]string, 0, len(session.players))
	for _, username := range session.players {
		authors = append(authors, nameAnonymousPlayer(username))
	}
	authorsList := strings.Join(authors, ", ")
	storyText := session.text
	session.alive = false
	session.mu.Unlock()

	m.bot.Send(tgbotapi.NewMessage(session.chatID, fmt.Sprintf("%s %s", msgStoryReport, authorsList)))
	m.bot.Send(tgbotapi.NewMessage(session.chatID, storyText))

	// Also post to the channel if configured
	if channelID != 0 {
		m.bot.Send(tgbotapi.NewMessage(channelID, storyText))
	}

	// Clean up
	m.storage.mu.Lock()
	delete(m.storage.sessions, session.chatID)
	m.storage.mu.Unlock()
}

func (s *GameSession) catchMessage(msg *tgbotapi.Message) {
	s.lastMessage = msg.Text
	s.text += " " + s.lastMessage
	s.waitingForReply = 0 // No longer waiting for a reply
}

func isLastTurn(counter, playerCount, maxTurns int) bool {
	if playerCount > maxTurns {
		return counter == playerCount
	}
	return counter == maxTurns
}

func trimLastPart(messageToSend, lastMessage string) string {
	lastMessageBySentences := strings.Split(strings.TrimRight(lastMessage, "."), ".")

	if len(lastMessageBySentences) > 1 {
		messageToSend += lastMessageBySentences[len(lastMessageBySentences)-1]
	} else {
		splitMessage := strings.Split(lastMessageBySentences[0], " ")
		if len(splitMessage) > 1 {
			messageToSend += "..." + strings.Join(splitMessage[len(splitMessage)/2:], " ")
		} else {
			messageToSend += "..." + lastMessageBySentences[0]
		}
	}

	if strings.HasSuffix(lastMessage, ".") {
		messageToSend += "."
	}

	return messageToSend
}

func nameAnonymousPlayer(username string) string {
	if username == "" {
		return fmt.Sprintf("%s %s", randomElement(adjectives), randomElement(animals))
	}
	return "@" + username
}

func randomElement(slice []string) string {
	return slice[rand.Intn(len(slice))]
}

func main() {
	// Random number generator is automatically seeded in Go 1.20+

	// Create a new bot API client
	token, err := os.ReadFile("/.tgtoken")
	if err != nil {
		fmt.Println("Error reading token:", err)
		return
	}

	bot, err := tgbotapi.NewBotAPI(strings.TrimSpace(string(token)))
	if err != nil {
		fmt.Println("Error creating bot:", err)
		return
	}

	// Create the module
	order := 100
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &order)
	}

	module := &Module{
		order:   order,
		storage: &GameStorage{sessions: make(map[int64]*GameSession)},
		bot:     bot,
	}

	// Start the HTTP server
	err = botModules.ServeModule(module, ":8080")
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}
