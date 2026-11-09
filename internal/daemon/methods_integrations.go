package daemon

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"

	"github.com/ryoku-dev/ryotunes/internal/integrations/listentogether"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/update"
)

func init() {
	register(protocol.MethodDiscordStatus, typed(discordStatus))
	register(protocol.MethodLastfmStatus, typed(lastfmStatus))
	register(protocol.MethodLastfmConnect, typed(lastfmConnect))
	register(protocol.MethodLastfmDisconnect, typed(lastfmDisconnect))
	register(protocol.MethodLTGetState, typed(ltGetState))
	register(protocol.MethodLTSetServerURL, typed(ltSetServerURL))
	register(protocol.MethodLTCreateRoom, typed(ltCreateRoom))
	register(protocol.MethodLTJoinRoom, typed(ltJoinRoom))
	register(protocol.MethodLTLeave, typed(ltLeave))
	register(protocol.MethodLTApproveJoin, typed(ltApproveJoin))
	register(protocol.MethodLTRejectJoin, typed(ltRejectJoin))
	register(protocol.MethodLTKick, typed(ltKick))
	register(protocol.MethodLTTransferHost, typed(ltTransferHost))
	register(protocol.MethodLTSuggest, typed(ltSuggest))
	register(protocol.MethodLTApproveSuggestion, typed(ltApproveSuggestion))
	register(protocol.MethodLTRejectSuggestion, typed(ltRejectSuggestion))
	register(protocol.MethodLTRequestSync, typed(ltRequestSync))
	register(protocol.MethodOpenExternal, typed(openExternal))
	register(protocol.MethodCheckForUpdates, typed(checkForUpdates))
	register(protocol.MethodInstallUpdate, typed(installUpdate))
	register(protocol.MethodRyokuThemeTokens, typed(ryokuThemeTokens))
}

func discordStatus(_ context.Context, a *App, _ protocol.Null) (model.DiscordStatus, error) {
	a.mu.Lock()
	client := a.Discord
	a.mu.Unlock()
	if client == nil {
		return model.DiscordStatus{Enabled: false, Status: "disabled"}, nil
	}
	return client.Status(), nil
}
func lastfmStatus(_ context.Context, a *App, _ protocol.Null) (model.LastfmStatus, error) {
	if a.Lastfm == nil {
		return model.LastfmStatus{}, nil
	}
	return a.Lastfm.Status(), nil
}
func lastfmConnect(ctx context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	if a.Lastfm == nil {
		return protocol.Null{}, fmt.Errorf("Last.fm is not configured.")
	}
	return protocol.Null{}, a.Lastfm.Connect(ctx)
}
func lastfmDisconnect(_ context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	if a.Lastfm == nil {
		return protocol.Null{}, nil
	}
	return protocol.Null{}, a.Lastfm.Disconnect()
}
func ltGetState(_ context.Context, a *App, _ protocol.Null) (model.LTState, error) {
	return a.LT.State(), nil
}
func ltSetServerURL(_ context.Context, a *App, p protocol.LTServerURLParams) (protocol.Null, error) {
	normalized, err := listentogether.ValidateServerURL(p.URL)
	if err != nil {
		return protocol.Null{}, err
	}
	if err := a.LT.SetServerURL(normalized); err != nil {
		return protocol.Null{}, err
	}
	if err := a.Store.KV().Set("lt_server_url", normalized); err != nil {
		return protocol.Null{}, err
	}
	return protocol.Null{}, nil
}
func ltCreateRoom(_ context.Context, a *App, p protocol.LTCreateRoomParams) (protocol.Null, error) {
	a.LT.CreateRoom(p.Username)
	return protocol.Null{}, nil
}
func ltJoinRoom(_ context.Context, a *App, p protocol.LTJoinRoomParams) (protocol.Null, error) {
	a.LT.JoinRoom(p.Code, p.Username)
	return protocol.Null{}, nil
}
func ltLeave(_ context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	a.LT.Leave()
	return protocol.Null{}, nil
}
func ltApproveJoin(_ context.Context, a *App, p protocol.LTUserParams) (protocol.Null, error) {
	a.LT.ApproveJoin(p.UserID)
	return protocol.Null{}, nil
}
func ltRejectJoin(_ context.Context, a *App, p protocol.LTUserParams) (protocol.Null, error) {
	a.LT.RejectJoin(p.UserID)
	return protocol.Null{}, nil
}
func ltKick(_ context.Context, a *App, p protocol.LTUserParams) (protocol.Null, error) {
	a.LT.Kick(p.UserID)
	return protocol.Null{}, nil
}
func ltTransferHost(_ context.Context, a *App, p protocol.LTUserParams) (protocol.Null, error) {
	a.LT.TransferHost(p.UserID)
	return protocol.Null{}, nil
}
func ltSuggest(_ context.Context, a *App, p protocol.LTSuggestParams) (protocol.Null, error) {
	a.LT.Suggest(p.Item)
	return protocol.Null{}, nil
}
func ltApproveSuggestion(_ context.Context, a *App, p protocol.LTSuggestionParams) (protocol.Null, error) {
	a.LT.ApproveSuggestion(p.ID)
	return protocol.Null{}, nil
}
func ltRejectSuggestion(_ context.Context, a *App, p protocol.LTSuggestionParams) (protocol.Null, error) {
	a.LT.RejectSuggestion(p.ID)
	return protocol.Null{}, nil
}
func ltRequestSync(_ context.Context, a *App, _ protocol.Null) (protocol.Null, error) {
	a.LT.RequestSync()
	return protocol.Null{}, nil
}
func openExternal(_ context.Context, _ *App, p protocol.URLParams) (protocol.Null, error) {
	if len(p.URL) > 4096 {
		return protocol.Null{}, fmt.Errorf("Link is too long to open safely.")
	}
	u, err := url.Parse(p.URL)
	if err != nil || u.Host == "" {
		return protocol.Null{}, fmt.Errorf("That is not a valid web link.")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return protocol.Null{}, fmt.Errorf("Only http:// and https:// links can be opened.")
	}
	if u.User != nil {
		return protocol.Null{}, fmt.Errorf("Links containing embedded credentials are not opened.")
	}
	if err := exec.Command("xdg-open", p.URL).Start(); err != nil {
		return protocol.Null{}, fmt.Errorf("Couldn't open the browser: %v", err)
	}
	return protocol.Null{}, nil
}
func checkForUpdates(ctx context.Context, _ *App, _ protocol.Null) (model.UpdateInfo, error) {
	return update.Check(ctx)
}
func installUpdate(ctx context.Context, _ *App, p protocol.InstallUpdateParams) (model.UpdateResult, error) {
	return update.Install(ctx, p.Version)
}
func ryokuThemeTokens(_ context.Context, _ *App, _ protocol.Null) (protocol.Null, error) {
	return protocol.Null{}, coded(protocol.ErrorClientSide, "the client reads the Ryoku palette singletons directly")
}
