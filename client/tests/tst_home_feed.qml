import QtQuick
import QtTest
import "../components"

TestCase {
    name: "HomeFeed"
    when: windowShown
    width: 600
    visible: true
    height: 500

    Component {
        id: feedComponent
        HomeFeed {
            id: feed
            width: 600
            height: 500
            property int headerHeight: 1600
            model: 20
            header: Rectangle { width: 600; height: feed.headerHeight; color: "gray" }
            delegate: Rectangle { width: 600; height: 200; color: "white" }
        }
    }

    function makeFeed() {
        var feed = createTemporaryObject(feedComponent, this);
        verify(feed !== null);
        waitForRendering(feed);
        tryCompare(feed.headerItem, "height", 1600);
        return feed;
    }

    function test_first_wheel_does_not_skip_tall_header() {
        var feed = makeFeed();
        var top = feed.contentY;
        mouseWheel(feed, 300, 200, 0, -120);
        tryVerify(function () { return feed.contentY > top; }, 1000, "Wheel must actually scroll");
        tryVerify(function () { return !feed.moving; });
        verify(feed.contentY - top < feed.height, "One wheel tick must not skip the whole Home header");
    }

    function test_releasing_pin_preserves_position() {
        var feed = makeFeed();
        var top = feed.contentY;
        feed.stickTop = false;
        compare(feed.contentY, top, "Disabling the pin must not restore pre-layout contentY=0");
    }

    function test_async_header_growth_stays_at_top_until_interaction() {
        var feed = makeFeed();
        feed.headerHeight = 2100;
        tryCompare(feed.headerItem, "height", 2100);
        tryCompare(feed, "contentY", feed.originY);
        feed.stickTop = false;
        feed.contentY += 120;
        var offset = feed.contentY;
        wait(50);
        compare(feed.contentY, offset, "The top pin must not reclaim a user-owned scroll position");
    }
}
