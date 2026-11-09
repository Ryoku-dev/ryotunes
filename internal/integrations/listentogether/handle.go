package listentogether

import (
	"context"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/ltproto"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func (c *Client) handle(ctx context.Context, generation uint64, message ltproto.ServerMessage) bool {
	switch value := message.(type) {
	case *ltproto.Pong:
		return false
	case *ltproto.RoomCreated:
		c.mu.Lock()
		c.state.Role = "host"
		c.state.Requesting = false
		userID := value.UserID
		c.state.MyID = &userID
		c.session = value.SessionToken
		c.absorbLocked(value.State)
		c.mu.Unlock()
		c.emitState()
		c.BroadcastSnapshot()
		return false
	case *ltproto.JoinRequest:
		isNew := false
		c.mu.Lock()
		for _, pending := range c.state.PendingJoins {
			if pending.UserID == value.UserID {
				c.mu.Unlock()
				return false
			}
		}
		c.state.PendingJoins = append(c.state.PendingJoins, model.LTPendingJoin{UserID: value.UserID, Username: value.Username})
		isNew = true
		c.mu.Unlock()
		if isNew {
			c.emitNotice(value.Username + " wants to join")
		}
	case *ltproto.JoinApproved:
		c.mu.Lock()
		c.state.Role = "guest"
		c.state.Requesting = false
		userID := value.UserID
		c.state.MyID = &userID
		c.session = value.SessionToken
		c.absorbLocked(value.State)
		c.mu.Unlock()
		c.applyState(ctx, value.State)
	case *ltproto.JoinRejected:
		c.emitNotice(value.Reason)
		c.closeLocally(generation, "")
		return true
	case *ltproto.Reconnected:
		c.mu.Lock()
		if value.IsHost {
			c.state.Role = "host"
		} else {
			c.state.Role = "guest"
		}
		c.state.Requesting = false
		userID := value.UserID
		c.state.MyID = &userID
		c.absorbLocked(value.State)
		c.mu.Unlock()
		if !value.IsHost {
			c.applyState(ctx, value.State)
		}
	case *ltproto.UserJoined:
		c.mu.Lock()
		pending := c.state.PendingJoins[:0]
		for _, join := range c.state.PendingJoins {
			if join.UserID != value.User.UserID {
				pending = append(pending, join)
			}
		}
		c.state.PendingJoins = pending
		found := false
		for index := range c.state.Users {
			if c.state.Users[index].UserID == value.User.UserID {
				c.state.Users[index] = value.User
				found = true
				break
			}
		}
		if !found {
			c.state.Users = append(c.state.Users, value.User)
		}
		c.mu.Unlock()
	case *ltproto.UserLeft:
		c.mu.Lock()
		users := c.state.Users[:0]
		for _, user := range c.state.Users {
			if user.UserID != value.UserID {
				users = append(users, user)
			}
		}
		c.state.Users = users
		c.mu.Unlock()
	case *ltproto.UserDisconnected:
		c.setUserConnected(value.UserID, false)
	case *ltproto.UserReconnected:
		c.setUserConnected(value.UserID, true)
	case *ltproto.HostChanged:
		becameHost := false
		c.mu.Lock()
		for index := range c.state.Users {
			c.state.Users[index].IsHost = c.state.Users[index].UserID == value.HostID
		}
		if c.state.MyID != nil && *c.state.MyID == value.HostID {
			c.state.Role = "host"
			becameHost = true
		} else {
			c.state.Role = "guest"
		}
		c.mu.Unlock()
		if becameHost {
			c.emitNotice("You're the host now.")
		}
	case *ltproto.SyncPlayback:
		c.mu.RLock()
		guest := c.state.Role == "guest"
		c.mu.RUnlock()
		if guest {
			c.applyPlayback(ctx, value.Playback)
		}
	case *ltproto.SyncState:
		c.mu.Lock()
		c.absorbLocked(value.State)
		guest := c.state.Role == "guest"
		c.mu.Unlock()
		if guest {
			c.applyState(ctx, value.State)
		}
	case *ltproto.SuggestionReceived:
		c.mu.Lock()
		c.state.Suggestions = append(c.state.Suggestions, value.Suggestion)
		c.mu.Unlock()
		if c.enqueueSuggestion(ctx, value.Suggestion) {
			c.send(&ltproto.ApproveSuggestion{ID: value.Suggestion.ID})
			c.broadcastQueue()
			c.removeSuggestion(value.Suggestion.ID)
			c.emitNotice(value.Suggestion.FromUsername + " added " + value.Suggestion.Track.Title)
		}
	case *ltproto.SuggestionApproved:
		c.emitNotice("Added to the session queue.")
	case *ltproto.SuggestionRejected:
		c.removeSuggestion(value.ID)
		c.emitNotice(value.Reason)
	case *ltproto.Kicked:
		c.emitNotice(value.Reason)
		c.closeLocally(generation, "")
		return true
	case *ltproto.Error:
		c.emitNotice(value.Message)
		c.mu.RLock()
		inRoom := c.state.Role != "none"
		c.mu.RUnlock()
		if !inRoom || value.Code == "session_expired" {
			c.closeLocally(generation, "")
			return true
		}
	}
	c.emitState()
	return false
}

func (c *Client) setUserConnected(userID string, connected bool) {
	c.mu.Lock()
	for index := range c.state.Users {
		if c.state.Users[index].UserID == userID {
			c.state.Users[index].IsConnected = connected
			break
		}
	}
	c.mu.Unlock()
}

func (c *Client) removeSuggestion(id string) {
	c.mu.Lock()
	remaining := c.state.Suggestions[:0]
	for _, suggestion := range c.state.Suggestions {
		if suggestion.ID != id {
			remaining = append(remaining, suggestion)
		}
	}
	c.state.Suggestions = remaining
	c.mu.Unlock()
}

func (c *Client) enqueueSuggestion(ctx context.Context, suggestion model.LTSuggestion) bool {
	if c.host == nil {
		return false
	}
	track := suggestion.Track
	queuedBy := suggestion.FromUsername
	track.QueuedBy = &queuedBy
	err := c.host.Enqueue(ctx, modelTrack(track), true)
	c.logHostError("enqueue suggestion", err)
	return err == nil
}

func (c *Client) applyState(ctx context.Context, state ltproto.RoomState) {
	if c.host == nil {
		return
	}
	if state.CurrentTrack != nil {
		c.logHostError("play synchronized track", c.host.PlayTrack(ctx, modelTrack(*state.CurrentTrack), ltproto.LivePositionMS(state, time.Now().UnixMilli()), state.IsPlaying))
	} else if state.IsPlaying {
		c.logHostError("resume synchronized playback", c.host.Resume(ctx))
	} else {
		c.logHostError("pause synchronized playback", c.host.Pause(ctx))
	}
	for _, track := range state.Queue {
		c.logHostError("enqueue synchronized track", c.host.Enqueue(ctx, modelTrack(track), false))
	}
}

func (c *Client) applyPlayback(ctx context.Context, playback ltproto.Playback) {
	if c.host == nil {
		return
	}
	c.mu.Lock()
	switch playback.Kind {
	case ltproto.PlaybackChangeTrack:
		c.state.CurrentTrack = playback.Track
		if playback.Queue != nil {
			c.state.Queue = append([]model.LTTrack(nil), (*playback.Queue)...)
		}
	case ltproto.PlaybackSyncQueue:
		if playback.Queue != nil {
			c.state.Queue = append([]model.LTTrack(nil), (*playback.Queue)...)
		}
	}
	c.mu.Unlock()
	switch playback.Kind {
	case ltproto.PlaybackPlay:
		position := playback.PositionMS
		if playback.ServerTimeMS > 0 {
			position += time.Now().UnixMilli() - playback.ServerTimeMS
		}
		c.logHostError("seek synchronized playback", c.host.Seek(ctx, position))
		c.logHostError("resume synchronized playback", c.host.Resume(ctx))
	case ltproto.PlaybackPause:
		c.logHostError("seek synchronized playback", c.host.Seek(ctx, playback.PositionMS))
		c.logHostError("pause synchronized playback", c.host.Pause(ctx))
	case ltproto.PlaybackSeek:
		c.logHostError("seek synchronized playback", c.host.Seek(ctx, playback.PositionMS))
	case ltproto.PlaybackChangeTrack:
		if playback.Track != nil {
			c.logHostError("change synchronized track", c.host.PlayTrack(ctx, modelTrack(*playback.Track), playback.PositionMS, playback.Playing))
		}
		if playback.Queue != nil {
			for _, track := range *playback.Queue {
				c.logHostError("enqueue synchronized track", c.host.Enqueue(ctx, modelTrack(track), false))
			}
		}
	case ltproto.PlaybackSyncQueue:
		if playback.Queue != nil {
			for _, track := range *playback.Queue {
				c.logHostError("enqueue synchronized track", c.host.Enqueue(ctx, modelTrack(track), false))
			}
		}
	}
}
