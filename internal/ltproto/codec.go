package ltproto

import (
	"encoding/json"
	"errors"
	"fmt"
)

var errNilMessage = errors.New("ltproto: nil message")

// EncodeClient encodes one client message with its canonical type tag.
func EncodeClient(message ClientMessage) ([]byte, error) {
	if message == nil {
		return nil, errNilMessage
	}
	setClientType(message)
	return json.Marshal(message)
}

// DecodeClient decodes one complete client message.
func DecodeClient(data []byte) (ClientMessage, error) {
	var tag struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, fmt.Errorf("decode client message tag: %w", err)
	}
	var message ClientMessage
	switch tag.Type {
	case "create_room":
		message = &CreateRoom{}
	case "join_room":
		message = &JoinRoom{}
	case "reconnect":
		message = &Reconnect{}
	case "leave_room":
		message = &LeaveRoom{}
	case "approve_join":
		message = &ApproveJoin{}
	case "reject_join":
		message = &RejectJoin{}
	case "kick_user":
		message = &KickUser{}
	case "transfer_host":
		message = &TransferHost{}
	case "playback":
		message = &PlaybackMessage{}
	case "suggest":
		message = &Suggest{}
	case "approve_suggestion":
		message = &ApproveSuggestion{}
	case "reject_suggestion":
		message = &RejectSuggestion{}
	case "request_sync":
		message = &RequestSync{}
	case "ping":
		message = &Ping{}
	default:
		return nil, fmt.Errorf("ltproto: unknown client message type %q", tag.Type)
	}
	if err := json.Unmarshal(data, message); err != nil {
		return nil, fmt.Errorf("decode client message %q: %w", tag.Type, err)
	}
	return message, nil
}

// EncodeServer encodes one server message with its canonical type tag.
func EncodeServer(message ServerMessage) ([]byte, error) {
	if message == nil {
		return nil, errNilMessage
	}
	setServerType(message)
	return json.Marshal(message)
}

// DecodeServer decodes one complete server message.
func DecodeServer(data []byte) (ServerMessage, error) {
	var tag struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, fmt.Errorf("decode server message tag: %w", err)
	}
	var message ServerMessage
	switch tag.Type {
	case "room_created":
		message = &RoomCreated{}
	case "join_request":
		message = &JoinRequest{}
	case "join_approved":
		message = &JoinApproved{}
	case "join_rejected":
		message = &JoinRejected{}
	case "reconnected":
		message = &Reconnected{}
	case "user_joined":
		message = &UserJoined{}
	case "user_left":
		message = &UserLeft{}
	case "user_disconnected":
		message = &UserDisconnected{}
	case "user_reconnected":
		message = &UserReconnected{}
	case "host_changed":
		message = &HostChanged{}
	case "sync_playback":
		message = &SyncPlayback{}
	case "sync_state":
		message = &SyncState{}
	case "suggestion_received":
		message = &SuggestionReceived{}
	case "suggestion_approved":
		message = &SuggestionApproved{}
	case "suggestion_rejected":
		message = &SuggestionRejected{}
	case "kicked":
		message = &Kicked{}
	case "error":
		message = &Error{}
	case "pong":
		message = &Pong{}
	default:
		return nil, fmt.Errorf("ltproto: unknown server message type %q", tag.Type)
	}
	if err := json.Unmarshal(data, message); err != nil {
		return nil, fmt.Errorf("decode server message %q: %w", tag.Type, err)
	}
	return message, nil
}

func setClientType(message ClientMessage) {
	switch value := message.(type) {
	case *CreateRoom:
		value.Type = "create_room"
	case *JoinRoom:
		value.Type = "join_room"
	case *Reconnect:
		value.Type = "reconnect"
	case *LeaveRoom:
		value.Type = "leave_room"
	case *ApproveJoin:
		value.Type = "approve_join"
	case *RejectJoin:
		value.Type = "reject_join"
	case *KickUser:
		value.Type = "kick_user"
	case *TransferHost:
		value.Type = "transfer_host"
	case *PlaybackMessage:
		value.Type = "playback"
	case *Suggest:
		value.Type = "suggest"
	case *ApproveSuggestion:
		value.Type = "approve_suggestion"
	case *RejectSuggestion:
		value.Type = "reject_suggestion"
	case *RequestSync:
		value.Type = "request_sync"
	case *Ping:
		value.Type = "ping"
	}
}

func setServerType(message ServerMessage) {
	switch value := message.(type) {
	case *RoomCreated:
		value.Type = "room_created"
	case *JoinRequest:
		value.Type = "join_request"
	case *JoinApproved:
		value.Type = "join_approved"
	case *JoinRejected:
		value.Type = "join_rejected"
	case *Reconnected:
		value.Type = "reconnected"
	case *UserJoined:
		value.Type = "user_joined"
	case *UserLeft:
		value.Type = "user_left"
	case *UserDisconnected:
		value.Type = "user_disconnected"
	case *UserReconnected:
		value.Type = "user_reconnected"
	case *HostChanged:
		value.Type = "host_changed"
	case *SyncPlayback:
		value.Type = "sync_playback"
	case *SyncState:
		value.Type = "sync_state"
	case *SuggestionReceived:
		value.Type = "suggestion_received"
	case *SuggestionApproved:
		value.Type = "suggestion_approved"
	case *SuggestionRejected:
		value.Type = "suggestion_rejected"
	case *Kicked:
		value.Type = "kicked"
	case *Error:
		value.Type = "error"
	case *Pong:
		value.Type = "pong"
	}
}
