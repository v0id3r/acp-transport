package acpv2

import (
	"encoding/json"
	"os"
	"testing"
)

func TestGeneratedMethodsMatchMeta(t *testing.T) {
	b, err := os.ReadFile("../../testdata/acp-schema-v2/meta.json")
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		Version         int               `json:"version"`
		AgentMethods    map[string]string `json:"agentMethods"`
		ClientMethods   map[string]string `json:"clientMethods"`
		ProtocolMethods map[string]string `json:"protocolMethods"`
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Version != ProtocolVersionNumber {
		t.Fatalf("ProtocolVersionNumber = %d, want %d", ProtocolVersionNumber, meta.Version)
	}
	if AgentMethodInitialize != meta.AgentMethods["initialize"] {
		t.Fatalf("AgentMethodInitialize = %q", AgentMethodInitialize)
	}
	if AgentMethodSessionPrompt != meta.AgentMethods["session_prompt"] {
		t.Fatalf("AgentMethodSessionPrompt = %q", AgentMethodSessionPrompt)
	}
	if AgentMethodToolCallCancel != meta.AgentMethods["tool_call_cancel"] {
		t.Fatalf("AgentMethodToolCallCancel = %q", AgentMethodToolCallCancel)
	}
	if ClientMethodSessionUpdate != meta.ClientMethods["session_update"] {
		t.Fatalf("ClientMethodSessionUpdate = %q", ClientMethodSessionUpdate)
	}
}

func TestGeneratedTypesMarshal(t *testing.T) {
	req := InitializeRequest{ProtocolVersion: ProtocolVersionNumber, Info: Implementation{Name: "test", Version: "0"}}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"info":{"name":"test","version":"0"},"protocolVersion":2}`
	if string(b) != want {
		t.Fatalf("InitializeRequest JSON = %s", b)
	}
	b, err = json.Marshal(SetSessionConfigOptionRequest{SessionID: "s", ConfigID: "model", Value: EncodeSessionConfigValue("gpt-6-sol")})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"configId":"model","sessionId":"s","value":"gpt-6-sol"}` {
		t.Fatalf("SetSessionConfigOptionRequest JSON = %s", b)
	}
	var decoded SetSessionConfigOptionRequest
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if id, ok := decoded.ValueID(); !ok || id != "gpt-6-sol" {
		t.Fatalf("ValueID = %q, %v", id, ok)
	}
}
