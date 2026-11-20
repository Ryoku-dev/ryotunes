import { convertFileSrc } from '@tauri-apps/api/core';

// Rewrite a Google image URL to (about) the pixel size a slot actually renders, so WebKitGTK
// doesn't decode a 544px (or 1080p) image for a 40px row. Only lh3/yt3 googleusercontent-style
// URLs carry the size in the URL (`=w544-h544` / `=s576` suffixes); anything else (notably
// i.ytimg.com path-variant thumbs, where other sizes can 404) is returned unchanged.
// Quantize requested thumbnail sizes to standard buckets so Google FIFE CDN hits standard
// cached sizes and does not fail with HTTP 500 on dynamic non-standard dimensions.
const BUCKETS = [64, 96, 120, 128, 160, 226, 256, 300, 320, 384, 512, 544, 640, 800, 1024, 1200];
function bucket(px: number): number {
	const n = Number(px);
	if (!Number.isFinite(n) || n <= 64) return 64;
	for (let i = 0; i < BUCKETS.length; i++) {
		if (BUCKETS[i] >= n) return BUCKETS[i];
	}
	return 1200;
}

export function thumb(url: string | undefined | null, px: number): string | undefined {
	if (!url) return undefined;
	// Local library artwork is a path on this machine, not a URL. The webview can't open a bare
	// path, so hand it through Tauri's asset protocol. Kept here rather than at the command
	// boundary so what gets stored (queue, Shortcuts) stays the real path — which is also what
	// MPRIS needs.
	if (url.startsWith('/') || /^[A-Za-z]:[\\/]/.test(url)) return convertFileSrc(url);
	const b = bucket(px);
	if (/=w\d+-h\d+/.test(url)) return url.replace(/=w\d+-h\d+/, `=w${b}-h${b}`);
	if (/=s\d+/.test(url)) return url.replace(/=s\d+/, `=s${b}`);
	return url;
}
