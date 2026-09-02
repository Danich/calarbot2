package settings

import "testing"

func TestUpsertChatKeepsFirstSeenAndUpdatesTitle(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertChat(Chat{ID: -1, Type: "group", Title: "старое", FirstSeen: 100, LastSeen: 100}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := s.UpsertChat(Chat{ID: -1, Type: "supergroup", Title: "новое", FirstSeen: 200, LastSeen: 200}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	got, err := s.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListChats returned %d chats; want 1", len(got))
	}
	if got[0].Title != "новое" || got[0].Type != "supergroup" {
		t.Errorf("chat = %q/%q; want \"новое\"/\"supergroup\"", got[0].Title, got[0].Type)
	}
	if got[0].FirstSeen != 100 {
		t.Errorf("FirstSeen = %d; want 100 — апсерт не должен его двигать", got[0].FirstSeen)
	}
	if got[0].LastSeen != 200 {
		t.Errorf("LastSeen = %d; want 200", got[0].LastSeen)
	}
}

func TestListChatsHidesLeftChats(t *testing.T) {
	s := newTestStore(t)

	for _, c := range []Chat{
		{ID: -1, Type: "group", Title: "группа", FirstSeen: 1, LastSeen: 1},
		{ID: 42, Type: "private", Title: "человек", FirstSeen: 1, LastSeen: 1},
		{ID: -2, Type: "group", Title: "турки", FirstSeen: 1, LastSeen: 1},
	} {
		if err := s.UpsertChat(c); err != nil {
			t.Fatalf("UpsertChat: %v", err)
		}
	}
	if err := s.MarkLeft(-2, 500); err != nil {
		t.Fatalf("MarkLeft: %v", err)
	}

	all, err := s.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListChats returned %d chats; want 2 — покинутый не показываем", len(all))
	}
	for _, c := range all {
		if c.ID == -2 {
			t.Error("покинутый чат остался в списке")
		}
	}
}

// Бота выкинули и добавили обратно: он снова там, значит пометка снимается.
func TestUpsertChatClearsLeftAt(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertChat(Chat{ID: -1, Type: "group", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := s.MarkLeft(-1, 500); err != nil {
		t.Fatalf("MarkLeft: %v", err)
	}
	if err := s.UpsertChat(Chat{ID: -1, Type: "group", FirstSeen: 600, LastSeen: 600}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	all, err := s.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListChats returned %d chats; want 1", len(all))
	}
	if all[0].LeftAt != nil {
		t.Errorf("LeftAt = %v; want nil", *all[0].LeftAt)
	}
}

func TestUpdateChatInfoLeavesTimestampsAlone(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertChat(Chat{ID: -1, Type: "group", Title: "", FirstSeen: 100, LastSeen: 100}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	if err := s.UpdateChatInfo(-1, "supergroup", "болталка", "chat"); err != nil {
		t.Fatalf("UpdateChatInfo: %v", err)
	}

	got, err := s.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListChats returned %d; want 1", len(got))
	}
	if got[0].Title != "болталка" || got[0].Type != "supergroup" || got[0].Username != "chat" {
		t.Errorf("chat = %+v; want болталка/supergroup/chat", got[0])
	}
	// Несущее: бэкфилл названий не должен выглядеть как активность в чате.
	if got[0].FirstSeen != 100 || got[0].LastSeen != 100 {
		t.Errorf("timestamps = %d/%d; want 100/100 — UpdateChatInfo их не трогает",
			got[0].FirstSeen, got[0].LastSeen)
	}
}
