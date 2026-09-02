package main

import (
	"errors"
	"path/filepath"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/settings"
)

func botWithSettings(t *testing.T) *Bot {
	t.Helper()
	s, err := settings.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return &Bot{SettingsStore: s}
}

func TestRecordChatStoresTitleAndType(t *testing.T) {
	b := botWithSettings(t)

	b.recordChat(&tgbotapi.Chat{ID: -1, Type: "supergroup", Title: "болталка"}, 1000)

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("ListChats returned %d; want 1", len(chats))
	}
	if chats[0].Title != "болталка" || chats[0].Type != "supergroup" {
		t.Errorf("chat = %+v; want болталка/supergroup", chats[0])
	}
}

// У лички нет title — там имя и @username. Иначе список личек в панели пуст.
func TestRecordChatNamesPrivateChatsByUser(t *testing.T) {
	b := botWithSettings(t)

	b.recordChat(&tgbotapi.Chat{ID: 42, Type: "private", FirstName: "Даня", UserName: "danich"}, 1000)

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("ListChats returned %d; want 1", len(chats))
	}
	if chats[0].Type != "private" {
		t.Fatalf("type = %q; want private", chats[0].Type)
	}
	if chats[0].Title != "Даня" || chats[0].Username != "danich" {
		t.Errorf("chat = %+v; want Даня/danich", chats[0])
	}
}

// Бота выгнали — канал должен уйти из панели, не дожидаясь чужих сообщений.
func TestRecordMembershipMarksKickedChatLeft(t *testing.T) {
	b := botWithSettings(t)
	b.recordChat(&tgbotapi.Chat{ID: -1, Type: "group", Title: "турки"}, 1000)

	b.recordMembership(&tgbotapi.ChatMemberUpdated{
		Chat:          tgbotapi.Chat{ID: -1, Type: "group", Title: "турки"},
		NewChatMember: tgbotapi.ChatMember{Status: "kicked"},
		Date:          2000,
	})

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 0 {
		t.Fatalf("ListChats returned %+v; want none — бота там больше нет", chats)
	}
}

func TestRecordMembershipAddsNewChat(t *testing.T) {
	b := botWithSettings(t)

	b.recordMembership(&tgbotapi.ChatMemberUpdated{
		Chat:          tgbotapi.Chat{ID: -7, Type: "group", Title: "новый"},
		NewChatMember: tgbotapi.ChatMember{Status: "member"},
		Date:          2000,
	})

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 || chats[0].ID != -7 {
		t.Fatalf("ListChats = %+v; want the new chat -7", chats)
	}
}

// Ограничили и заодно исключили из чата — это тоже "бота там больше нет",
// иначе панель держит его в списке навечно, а её кнопка "выйти" на таком
// чате падает: leaveChat не работает там, где бота уже нет.
func TestRecordMembershipMarksRestrictedNonMemberChatLeft(t *testing.T) {
	b := botWithSettings(t)
	b.recordChat(&tgbotapi.Chat{ID: -1, Type: "group", Title: "турки"}, 1000)

	b.recordMembership(&tgbotapi.ChatMemberUpdated{
		Chat:          tgbotapi.Chat{ID: -1, Type: "group", Title: "турки"},
		NewChatMember: tgbotapi.ChatMember{Status: "restricted", IsMember: false},
		Date:          2000,
	})

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 0 {
		t.Fatalf("ListChats returned %+v; want none — бота там больше нет", chats)
	}
}

// Ограничили, но бот всё ещё в чате — просто притих. Он должен остаться в
// панели: leaveChat на нём сработает как обычно.
func TestRecordMembershipKeepsRestrictedMemberChatListed(t *testing.T) {
	b := botWithSettings(t)
	b.recordChat(&tgbotapi.Chat{ID: -1, Type: "group", Title: "турки"}, 1000)

	b.recordMembership(&tgbotapi.ChatMemberUpdated{
		Chat:          tgbotapi.Chat{ID: -1, Type: "group", Title: "турки"},
		NewChatMember: tgbotapi.ChatMember{Status: "restricted", IsMember: true},
		Date:          2000,
	})

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("ListChats returned %+v; want the chat still listed — бот там остался", chats)
	}
}

// Ни имени, ни фамилии, ни username — бывает у удалённых аккаунтов. Без
// запасного варианта title остался бы пустым, и строка в списке личек в
// панели была бы неотличима от других пустых строк.
func TestRecordChatNamesNamelessPrivateChatByID(t *testing.T) {
	b := botWithSettings(t)

	b.recordChat(&tgbotapi.Chat{ID: 99, Type: "private"}, 1000)

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("ListChats returned %d; want 1", len(chats))
	}
	if chats[0].Title == "" {
		t.Errorf("chat = %+v; want a non-empty title", chats[0])
	}
}

// fakeChatInfo — телеграм для бэкфилла названий, без сети.
type fakeChatInfo struct {
	chats map[int64]tgbotapi.Chat
	errs  map[int64]error
	asked []int64
}

func (f *fakeChatInfo) GetChat(c tgbotapi.ChatInfoConfig) (tgbotapi.Chat, error) {
	f.asked = append(f.asked, c.ChatID)
	if err, ok := f.errs[c.ChatID]; ok {
		return tgbotapi.Chat{}, err
	}
	return f.chats[c.ChatID], nil
}

func TestBackfillNamesChatsThatHaveNoTitle(t *testing.T) {
	b := botWithSettings(t)
	if err := b.SettingsStore.UpsertChat(settings.Chat{ID: -1, Type: "group", Title: "", FirstSeen: 100, LastSeen: 100}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	api := &fakeChatInfo{chats: map[int64]tgbotapi.Chat{
		-1: {ID: -1, Type: "supergroup", Title: "болталка"},
	}}
	b.backfillChatTitles(api)

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if chats[0].Title != "болталка" || chats[0].Type != "supergroup" {
		t.Errorf("chat = %+v; want болталка/supergroup", chats[0])
	}
	// Бэкфилл — не активность: иначе чат всплывёт наверх списка и соврёт
	// в колонке «последняя активность».
	if chats[0].LastSeen != 100 {
		t.Errorf("LastSeen = %d; want 100", chats[0].LastSeen)
	}
}

func TestBackfillSkipsChatsThatAlreadyHaveATitle(t *testing.T) {
	b := botWithSettings(t)
	if err := b.SettingsStore.UpsertChat(settings.Chat{ID: -1, Type: "group", Title: "уже есть", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	api := &fakeChatInfo{}
	b.backfillChatTitles(api)

	if len(api.asked) != 0 {
		t.Errorf("спросили телеграм про %v; названия уже есть, спрашивать не о чем", api.asked)
	}
}

// Бота могли выгнать из чата — getChat тогда откажет. Это не повод не
// запускаться и не повод бросить остальные чаты.
func TestBackfillSurvivesAFailedChat(t *testing.T) {
	b := botWithSettings(t)
	for _, id := range []int64{-1, -2} {
		if err := b.SettingsStore.UpsertChat(settings.Chat{ID: id, Type: "group", FirstSeen: 1, LastSeen: 1}); err != nil {
			t.Fatalf("UpsertChat: %v", err)
		}
	}

	api := &fakeChatInfo{
		chats: map[int64]tgbotapi.Chat{-2: {ID: -2, Type: "group", Title: "живой"}},
		errs:  map[int64]error{-1: errors.New("chat not found")},
	}
	b.backfillChatTitles(api)

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	byID := map[int64]settings.Chat{}
	for _, c := range chats {
		byID[c.ID] = c
	}
	if byID[-2].Title != "живой" {
		t.Errorf("чат -2 = %q; отказ по соседу не должен был его пропустить", byID[-2].Title)
	}
	if byID[-1].Title != "" {
		t.Errorf("чат -1 = %q; при отказе название остаётся пустым", byID[-1].Title)
	}
}

// У лички нет title — имя собирается из имени и фамилии.
func TestBackfillNamesPrivateChats(t *testing.T) {
	b := botWithSettings(t)
	if err := b.SettingsStore.UpsertChat(settings.Chat{ID: 42, Type: "private", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	api := &fakeChatInfo{chats: map[int64]tgbotapi.Chat{
		42: {ID: 42, Type: "private", FirstName: "Даня", UserName: "danich"},
	}}
	b.backfillChatTitles(api)

	chats, _ := b.SettingsStore.ListChats()
	if chats[0].Title != "Даня" || chats[0].Username != "danich" {
		t.Errorf("chat = %+v; want Даня/danich", chats[0])
	}
}
