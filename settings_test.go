package main

import "testing"

func TestUpdateSettingsMerges(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := &MalachiService{}
	if st, err := m.GetSettings(); err != nil || st != (Settings{}) {
		t.Fatalf("fresh settings = %+v, %v", st, err)
	}
	if _, err := m.UpdateSettings(Settings{Model: "opencode-go/kimi"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.UpdateSettings(Settings{Thinking: "high"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.UpdateSettings(Settings{CodeMode: "explore"}); err != nil {
		t.Fatal(err)
	}
	st, err := m.GetSettings()
	if err != nil || st.Model != "opencode-go/kimi" || st.Thinking != "high" || st.CodeMode != "explore" {
		t.Fatalf("settings = %+v, %v", st, err)
	}
}
