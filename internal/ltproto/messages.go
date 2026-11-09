package ltproto

// ClientMessage is a message accepted by the relay.
type ClientMessage interface {
	clientMessage()
}

// CreateRoom asks the relay to create a room for Username.
type CreateRoom struct {
	Type     string `json:"type"`
	Username string `json:"username"`
}

// JoinRoom asks the room host to admit Username.
type JoinRoom struct {
	Type     string `json:"type"`
	RoomCode string `json:"room_code"`
	Username string `json:"username"`
}

// Reconnect resumes a disconnected participant session.
type Reconnect struct {
	Type         string `json:"type"`
	SessionToken string `json:"session_token"`
}

// LeaveRoom permanently leaves the current room.
type LeaveRoom struct {
	Type string `json:"type"`
}

// ApproveJoin admits a pending user.
type ApproveJoin struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
}

// RejectJoin rejects a pending user.
type RejectJoin struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
}

// KickUser removes an admitted user.
type KickUser struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
}

// TransferHost hands room authority to another user.
type TransferHost struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
}

// PlaybackMessage carries one host playback action.
type PlaybackMessage struct {
	Type string `json:"type"`
	Playback
}

// Suggest submits a guest track to the room host.
type Suggest struct {
	Type  string `json:"type"`
	Track Track  `json:"track"`
}

// ApproveSuggestion accepts a pending suggestion.
type ApproveSuggestion struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// RejectSuggestion rejects a pending suggestion.
type RejectSuggestion struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// RequestSync asks for the authoritative room state.
type RequestSync struct {
	Type string `json:"type"`
}

// Ping is an application-level keepalive.
type Ping struct {
	Type string `json:"type"`
}

func (*CreateRoom) clientMessage()        {}
func (*JoinRoom) clientMessage()          {}
func (*Reconnect) clientMessage()         {}
func (*LeaveRoom) clientMessage()         {}
func (*ApproveJoin) clientMessage()       {}
func (*RejectJoin) clientMessage()        {}
func (*KickUser) clientMessage()          {}
func (*TransferHost) clientMessage()      {}
func (*PlaybackMessage) clientMessage()   {}
func (*Suggest) clientMessage()           {}
func (*ApproveSuggestion) clientMessage() {}
func (*RejectSuggestion) clientMessage()  {}
func (*RequestSync) clientMessage()       {}
func (*Ping) clientMessage()              {}

// ServerMessage is a message emitted by the relay.
type ServerMessage interface {
	serverMessage()
}

// RoomCreated confirms a newly created host session.
type RoomCreated struct {
	Type         string    `json:"type"`
	RoomCode     string    `json:"room_code"`
	UserID       string    `json:"user_id"`
	SessionToken string    `json:"session_token"`
	State        RoomState `json:"state"`
}

// JoinRequest informs a host that a user is waiting.
type JoinRequest struct {
	Type     string `json:"type"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

// JoinApproved confirms admission to a room.
type JoinApproved struct {
	Type         string    `json:"type"`
	UserID       string    `json:"user_id"`
	SessionToken string    `json:"session_token"`
	State        RoomState `json:"state"`
}

// JoinRejected reports that a room join failed.
type JoinRejected struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// Reconnected confirms that a session was resumed.
type Reconnected struct {
	Type   string    `json:"type"`
	UserID string    `json:"user_id"`
	IsHost bool      `json:"is_host"`
	State  RoomState `json:"state"`
}

// UserJoined announces an admitted participant.
type UserJoined struct {
	Type string `json:"type"`
	User User   `json:"user"`
}

// UserLeft announces a permanent departure.
type UserLeft struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
}

// UserDisconnected announces a resumable disconnection.
type UserDisconnected struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
}

// UserReconnected announces a resumed connection.
type UserReconnected struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
}

// HostChanged announces a room authority handoff.
type HostChanged struct {
	Type   string `json:"type"`
	HostID string `json:"host_id"`
}

// SyncPlayback delivers a host playback action.
type SyncPlayback struct {
	Type string `json:"type"`
	Playback
}

// SyncState delivers the complete authoritative room state.
type SyncState struct {
	Type  string    `json:"type"`
	State RoomState `json:"state"`
}

// SuggestionReceived delivers a guest suggestion to the host.
type SuggestionReceived struct {
	Type       string     `json:"type"`
	Suggestion Suggestion `json:"suggestion"`
}

// SuggestionApproved informs a guest that its suggestion was accepted.
type SuggestionApproved struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// SuggestionRejected informs a guest that its suggestion was declined.
type SuggestionRejected struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Kicked reports removal from the room.
type Kicked struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// Error reports a relay refusal.
type Error struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Pong acknowledges an application-level keepalive.
type Pong struct {
	Type string `json:"type"`
}

func (*RoomCreated) serverMessage()        {}
func (*JoinRequest) serverMessage()        {}
func (*JoinApproved) serverMessage()       {}
func (*JoinRejected) serverMessage()       {}
func (*Reconnected) serverMessage()        {}
func (*UserJoined) serverMessage()         {}
func (*UserLeft) serverMessage()           {}
func (*UserDisconnected) serverMessage()   {}
func (*UserReconnected) serverMessage()    {}
func (*HostChanged) serverMessage()        {}
func (*SyncPlayback) serverMessage()       {}
func (*SyncState) serverMessage()          {}
func (*SuggestionReceived) serverMessage() {}
func (*SuggestionApproved) serverMessage() {}
func (*SuggestionRejected) serverMessage() {}
func (*Kicked) serverMessage()             {}
func (*Error) serverMessage()              {}
func (*Pong) serverMessage()               {}
