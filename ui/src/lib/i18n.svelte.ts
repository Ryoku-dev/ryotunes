// Reactive UI language for the Ryotunes chrome. Content coming from the
// InnerTube API is already localised by the backend session locale (hl/gl);
// this module only covers the hard-coded Svelte strings (context menus,
// dialogs, settings, status text), which the API never sees.
import fr from './i18n/fr.json';

const LOCALES: Record<string, Record<string, string>> = { fr };

function detect(): string {
	const nav = (globalThis.navigator?.language ?? 'en').toLowerCase();
	return nav.startsWith('fr') ? 'fr' : 'en';
}

/** Current UI language ('fr' | 'en'). */
export const locale = $state(detect());

/**
 * Translate a hard-coded English chrome string. Unknown keys return the
 * source text unchanged (English), so a missing translation degrades
 * gracefully instead of showing a key name.
 */
export function t(key: string): string {
	return LOCALES[locale]?.[key] ?? key;
}
