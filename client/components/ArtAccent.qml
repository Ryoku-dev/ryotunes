import QtQuick
import "../"

// Samples the playing cover's accent into Playback.artAccent: the Svelte artworkAccent algorithm
// (a 24 px downsample, colours bucketed by 3 bits per channel, the bucket with the best
// saturation-times-mid-lightness score wins). One instance per window, because a Canvas only
// paints inside a window that renders: whichever window is visible keeps the accent current, and
// both computing the same URL is a no-op. Invisible, 24 x 24, painted once per track.
Canvas {
    id: root

    readonly property string url: (Playback.now && Playback.now.thumbnail) ? Style.thumb(Playback.now.thumbnail, 64) : ""
    property string sampled: ""

    width: 24
    height: 24
    visible: false
    renderStrategy: Canvas.Immediate
    renderTarget: Canvas.Image

    onUrlChanged: {
        if (!root.url) {
            Playback.artAccent = "transparent";
            root.sampled = "";
            return;
        }
        if (root.isImageLoaded(root.url))
            root.requestPaint();
        else
            root.loadImage(root.url);
    }
    onImageLoaded: if (root.url && root.isImageLoaded(root.url)) root.requestPaint()
    Component.onCompleted: if (root.url) root.loadImage(root.url)

    onPaint: {
        if (!root.url || root.sampled === root.url || !root.isImageLoaded(root.url))
            return;
        var ctx = getContext("2d");
        ctx.clearRect(0, 0, width, height);
        ctx.drawImage(root.url, 0, 0, width, height);
        var data = ctx.getImageData(0, 0, width, height).data;
        var buckets = {};
        var best = null;
        for (var i = 0; i < data.length; i += 4) {
            if (data[i + 3] < 128) continue;
            var r = data[i], g = data[i + 1], b = data[i + 2];
            var max = Math.max(r, g, b) / 255, min = Math.min(r, g, b) / 255;
            var sat = max === 0 ? 0 : (max - min) / max;
            if (sat < 0.12) continue;
            var score = sat * (1 - Math.abs(max - 0.65));
            var key = ((r >> 5) << 6) | ((g >> 5) << 3) | (b >> 5);
            var cur = buckets[key] || (buckets[key] = { n: 0, r: 0, g: 0, b: 0, score: 0 });
            cur.n++; cur.r += r; cur.g += g; cur.b += b; cur.score += score;
            if (!best || cur.score > best.score) best = cur;
        }
        root.sampled = root.url;
        Playback.artAccent = best ? Qt.rgba(best.r / best.n / 255, best.g / best.n / 255, best.b / best.n / 255, 1) : "transparent";
    }
}
