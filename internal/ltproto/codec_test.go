package ltproto

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestClientMessageRoundTrips(t *testing.T) {
	track := Track{ID: "video", Title: "Title", Artist: "Artist", DurationMS: 1234}
	messages := []ClientMessage{
		&CreateRoom{Username: "Host"},
		&JoinRoom{RoomCode: "12345678", Username: "Guest"},
		&Reconnect{SessionToken: "token"},
		&LeaveRoom{},
		&ApproveJoin{UserID: "user"},
		&RejectJoin{UserID: "user"},
		&KickUser{UserID: "user"},
		&TransferHost{UserID: "user"},
		&PlaybackMessage{Playback: Playback{Kind: PlaybackChangeTrack, PositionMS: 42, Track: &track, Queue: &[]Track{track}, Playing: true, Volume: 1}},
		&Suggest{Track: track},
		&ApproveSuggestion{ID: "suggestion"},
		&RejectSuggestion{ID: "suggestion"},
		&RequestSync{},
		&Ping{},
	}
	for _, message := range messages {
		encoded, err := EncodeClient(message)
		if err != nil {
			t.Fatalf("EncodeClient(%T): %v", message, err)
		}
		decoded, err := DecodeClient(encoded)
		if err != nil {
			t.Fatalf("DecodeClient(%T): %v", message, err)
		}
		if !reflect.DeepEqual(message, decoded) {
			t.Errorf("round trip %T = %#v, want %#v", message, decoded, message)
		}
	}
}

func TestServerMessageRoundTrips(t *testing.T) {
	track := Track{ID: "video", Title: "Title", Artist: "Artist", DurationMS: 1234}
	user := User{UserID: "user", Username: "Guest", IsConnected: true}
	state := RoomState{RoomCode: "12345678", HostID: "host", Users: []User{user}, CurrentTrack: &track, Volume: 1, Queue: []Track{track}}
	suggestion := Suggestion{ID: "suggestion", FromUserID: "user", FromUsername: "Guest", Track: track}
	messages := []ServerMessage{
		&RoomCreated{RoomCode: state.RoomCode, UserID: "host", SessionToken: "token", State: state},
		&JoinRequest{UserID: "user", Username: "Guest"},
		&JoinApproved{UserID: "user", SessionToken: "token", State: state},
		&JoinRejected{Reason: "reason"},
		&Reconnected{UserID: "user", IsHost: true, State: state},
		&UserJoined{User: user},
		&UserLeft{UserID: "user"},
		&UserDisconnected{UserID: "user"},
		&UserReconnected{UserID: "user"},
		&HostChanged{HostID: "user"},
		&SyncPlayback{Playback: Playback{Kind: PlaybackPlay, PositionMS: 42}},
		&SyncState{State: state},
		&SuggestionReceived{Suggestion: suggestion},
		&SuggestionApproved{ID: "suggestion"},
		&SuggestionRejected{ID: "suggestion", Reason: "reason"},
		&Kicked{Reason: "reason"},
		&Error{Code: "code", Message: "message"},
		&Pong{},
	}
	for _, message := range messages {
		encoded, err := EncodeServer(message)
		if err != nil {
			t.Fatalf("EncodeServer(%T): %v", message, err)
		}
		decoded, err := DecodeServer(encoded)
		if err != nil {
			t.Fatalf("DecodeServer(%T): %v", message, err)
		}
		if !reflect.DeepEqual(message, decoded) {
			t.Errorf("round trip %T = %#v, want %#v", message, decoded, message)
		}
	}
}

func TestPlaybackMessageIsFlat(t *testing.T) {
	emptyQueue := []Track{}
	encoded, err := EncodeClient(&PlaybackMessage{Playback: Playback{Kind: PlaybackSeek, PositionMS: 99, Queue: &emptyQueue}})
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	if _, exists := object["playback"]; exists {
		t.Fatal("playback payload was nested")
	}
	if string(object["type"]) != `"playback"` || string(object["kind"]) != `"seek"` || string(object["queue"]) != `[]` {
		t.Fatalf("unexpected playback JSON: %s", encoded)
	}
}
