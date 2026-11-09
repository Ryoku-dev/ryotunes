package youtube

import "github.com/ryoku-dev/ryotunes/internal/model"

type clientConfig struct {
	key, name, version, id, userAgent                            string
	osName, osVersion, deviceMake, deviceModel                   string
	androidSDK, buildID, cronetVersion, packageName              string
	friendlyName                                                 string
	embedded, loginSupported, loginRequired, signature, poTokens bool
}

var clients = map[string]clientConfig{
	"WEB_REMIX":          {key: "WEB_REMIX", name: "WEB_REMIX", version: "1.20260213.01.00", id: "67", userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0", loginSupported: true, signature: true, poTokens: true},
	"WEB_CREATOR":        {key: "WEB_CREATOR", name: "WEB_CREATOR", version: "1.20220726.00.00", id: "62", userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0", loginSupported: true, loginRequired: true, signature: true, poTokens: true},
	"TVHTML5":            {key: "TVHTML5", name: "TVHTML5", version: "7.20260707.07.00", id: "7", userAgent: "Mozilla/5.0 (ChromiumStylePlatform) Cobalt/25.lts.30.1034943-gold (unlike Gecko), Unknown_TV_Unknown_0/Unknown (Unknown; Unknown)", loginSupported: true, signature: true},
	"VISIONOS":           {key: "VISIONOS", name: "VISIONOS", version: "0.1", id: "101", userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15", osName: "visionOS", osVersion: "1.3.21O771", deviceMake: "Apple", deviceModel: "RealityDevice14,1", friendlyName: "visionOS"},
	"ANDROID_VR_1_43_32": {key: "ANDROID_VR_1_43_32", name: "ANDROID_VR", version: "1.43.32", id: "28", userAgent: "com.google.android.apps.youtube.vr.oculus/1.43.32 (Linux; U; Android 12; en_US; Quest 3; Build/SQ3A.220605.009.A1; Cronet/107.0.5284.2)", osName: "Android", osVersion: "12", deviceMake: "Oculus", deviceModel: "Quest 3", androidSDK: "32", buildID: "SQ3A.220605.009.A1", cronetVersion: "107.0.5284.2", packageName: "com.google.android.apps.youtube.vr.oculus", friendlyName: "Android VR 1.43"},
	"IOS_MUSIC":          {key: "IOS_MUSIC", name: "IOS_MUSIC", version: "7.01.05", id: "26", userAgent: "com.google.ios.youtubemusic/7.01.05 (iPhone16,2; U; CPU iOS 18_2 like Mac OS X;)", osName: "iOS", osVersion: "18.2.22C152", deviceMake: "Apple", deviceModel: "iPhone16,2", friendlyName: "iOS Music"},
	"ANDROID_VR_1_65_10": {key: "ANDROID_VR_1_65_10", name: "ANDROID_VR", version: "1.65.10", id: "28", userAgent: "com.google.android.apps.youtube.vr.oculus/1.65.10 (Linux; U; Android 12L; eureka-user Build/SQ3A.220605.009.A1) gzip", osName: "Android", osVersion: "12L", deviceMake: "Oculus", deviceModel: "Quest 3", androidSDK: "32", friendlyName: "Android VR 1.65"},
}

var ordinaryStreamClients = []model.StreamClient{
	model.StreamClientWebRemix,
	model.StreamClientVisionOS,
	model.StreamClientAndroidVR165,
	model.StreamClientAndroidVR143,
}

var uploadStreamClients = []model.StreamClient{
	model.StreamClientTVHTML5,
	model.StreamClientWebCreator,
}

func (c clientConfig) context(visitor, dataSync, cookie, videoID string) map[string]any {
	cm := map[string]any{"clientName": c.name, "clientVersion": c.version, "gl": localeRegion(), "hl": localeLanguage()}
	optionalString(cm, "osName", c.osName)
	optionalString(cm, "osVersion", c.osVersion)
	optionalString(cm, "deviceMake", c.deviceMake)
	optionalString(cm, "deviceModel", c.deviceModel)
	optionalString(cm, "androidSdkVersion", c.androidSDK)
	optionalString(cm, "buildId", c.buildID)
	optionalString(cm, "cronetVersion", c.cronetVersion)
	optionalString(cm, "packageName", c.packageName)
	optionalString(cm, "friendlyName", c.friendlyName)
	optionalString(cm, "visitorData", visitor)
	ctx := map[string]any{
		"client":  cm,
		"request": map[string]any{"internalExperimentFlags": []any{}, "useSsl": true},
		"user":    map[string]any{"lockedSafetyMode": false},
	}
	if c.loginSupported && cookie != "" && dataSync != "" {
		ctx["user"].(map[string]any)["onBehalfOfUser"] = dataSync
	}
	if c.embedded && videoID != "" {
		ctx["thirdParty"] = map[string]any{"embedUrl": "https://www.youtube.com/watch?v=" + videoID}
	}
	return ctx
}

func optionalString(dst map[string]any, key, value string) {
	if value != "" {
		dst[key] = value
	}
}
