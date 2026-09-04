//! The `context` object sent in every request body. write API, Metrolist `Context.kt`.

use serde::Serialize;

use crate::clients::YouTubeClient;

/// Locale: `gl` = country code, `hl` = BCP-47 language tag. write API.
#[derive(Debug, Clone, Serialize)]
pub struct Locale {
    pub gl: String,
    pub hl: String,
}

impl Default for Locale {
    fn default() -> Self {
        Locale { gl: "US".into(), hl: "en".into() }
    }
}

impl Locale {
    /// Derive a `Locale` from the process environment (`LC_ALL`/`LANG`), so the
    /// YouTube API answers in the user's language instead of always US/en.
    /// Falls back to [`Locale::default`] when nothing parseable is set.
    pub fn from_env() -> Self {
        let raw = std::env::var("LC_ALL").or_else(|_| std::env::var("LANG")).unwrap_or_default();
        Self::from_lang(&raw)
    }

    fn from_lang(raw: &str) -> Self {
        // "fr_FR.UTF-8" -> ("fr", "FR"); "", "C" or "POSIX" -> default.
        let base = raw.split('.').next().unwrap_or("");
        let mut parts = base.split('_');
        let lang = parts.next().unwrap_or("").to_ascii_lowercase();
        if lang.is_empty() || lang == "c" || lang == "posix" {
            return Locale::default();
        }
        let terr = parts.next().unwrap_or("").to_ascii_uppercase();
        let hl = match (lang.as_str(), terr.as_str()) {
            // Language+territory pairs YouTube localises as a compound tag.
            ("zh", "CN") => "zh-CN".into(),
            ("zh", "TW") => "zh-TW".into(),
            ("zh", "HK") => "zh-HK".into(),
            ("pt", "BR") => "pt-BR".into(),
            ("pt", "PT") => "pt-PT".into(),
            ("es", "US") => "es-US".into(),
            ("es", "ES") => "es".into(),
            ("es", _) => "es-419".into(),
            ("en", "GB") => "en-GB".into(),
            ("fr", "CA") => "fr-CA".into(),
            _ => lang,
        };
        Locale { gl: if terr.is_empty() { "US".into() } else { terr }, hl }
    }
}

// The three load-bearing JSON flags (write API) are realized structurally here:
// - ignoreUnknownKeys → serde ignores unknown fields on Deserialize by default.
// - explicitNulls = false → `skip_serializing_if = "Option::is_none"` on every Option.
// - encodeDefaults = true → non-Option fields are always emitted (e.g. useSsl, contentCheckOk).

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Context {
    pub client: Client,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub third_party: Option<ThirdParty>,
    pub request: Request,
    pub user: User,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Client {
    pub client_name: String,
    pub client_version: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub os_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub os_version: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub device_make: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub device_model: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub android_sdk_version: Option<String>,
    pub gl: String,
    pub hl: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub visitor_data: Option<String>,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ThirdParty {
    pub embed_url: String,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Request {
    pub internal_experiment_flags: Vec<String>,
    pub use_ssl: bool,
}

impl Default for Request {
    fn default() -> Self {
        Request { internal_experiment_flags: Vec::new(), use_ssl: true }
    }
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct User {
    pub locked_safety_mode: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub on_behalf_of_user: Option<String>,
}

impl YouTubeClient {
    /// Build the `context` object for this client. Port of `YouTubeClient.toContext`.
    /// `on_behalf_of_user` (dataSyncId) is set only when the client supports login.
    pub fn to_context(
        &self,
        locale: &Locale,
        visitor_data: Option<&str>,
        data_sync_id: Option<&str>,
    ) -> Context {
        Context {
            client: Client {
                client_name: self.client_name.clone(),
                client_version: self.client_version.clone(),
                os_name: self.os_name.clone(),
                os_version: self.os_version.clone(),
                device_make: self.device_make.clone(),
                device_model: self.device_model.clone(),
                android_sdk_version: self.android_sdk_version.clone(),
                gl: locale.gl.clone(),
                hl: locale.hl.clone(),
                visitor_data: visitor_data.map(str::to_owned),
            },
            third_party: self.is_embedded.then(|| ThirdParty {
                // embedUrl is filled per-video by the /player builder for embedded clients.
                embed_url: String::new(),
            }),
            request: Request::default(),
            user: User {
                locked_safety_mode: false,
                on_behalf_of_user: if self.login_supported {
                    data_sync_id.map(str::to_owned)
                } else {
                    None
                },
            },
        }
    }
}
