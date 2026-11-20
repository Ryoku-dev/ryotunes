import QtQuick
import QtTest
import "../lib/style.js" as Style

// Style's thumb()/fmtTime() are pure and live in lib/style.js precisely so this test can reach them:
// the Style singleton imports Tokens, which imports Quickshell, and a qmltestrunner process cannot
// load the Quickshell plugin. Importing the module directly tests the exact code Style re-exposes.
TestCase {
    name: "Style"

    function test_thumb_rewrites_sizes() {
        compare(Style.thumb("https://x/a=w120-h120-l90", 544), "https://x/a=w544-h544-l90");
        compare(Style.thumb("https://x/a=w120-h120-l90", 708), "https://x/a=w800-h800-l90");
        compare(Style.thumb("https://x/a=s200", 64), "https://x/a=s64");
        compare(Style.thumb("/music/cover.jpg", 64), "file:///music/cover.jpg");
        compare(Style.thumb("", 64), undefined);
    }

    function test_fmtTime() {
        compare(Style.fmtTime(0), "0:00");
        compare(Style.fmtTime(65.9), "1:05");
        compare(Style.fmtTime(3600), "60:00");
    }
}
