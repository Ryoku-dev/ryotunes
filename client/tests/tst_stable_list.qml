import QtQuick
import QtTest
import "../components"

TestCase {
    name: "StableList"
    when: windowShown
    width: 600
    height: 500
    visible: true

    function songs(from, to) {
        var out = [];
        for (var i = from; i < to; i++)
            out.push({ id: "v" + i });
        return out;
    }

    Component {
        id: listComponent
        StableList {
            rowKey: (row) => row.id
            width: 600
            height: 500
            header: Rectangle { width: 600; height: 160; color: "gray" }
            delegate: Rectangle { required property var modelData; width: 600; height: 60; color: "white" }
        }
    }

    function makeList(rows) {
        var list = createTemporaryObject(listComponent, this, { rows: rows });
        verify(list !== null);
        waitForRendering(list);
        return list;
    }

    function test_appending_a_page_keeps_the_scroll_position() {
        var first = songs(0, 100);
        var list = makeList(first);
        list.contentY = 3000;
        compare(list.contentY, 3000);
        list.rows = first.concat(songs(100, 200));
        wait(150);
        compare(list.count, 200);
        compare(list.contentY, 3000);
    }

    function test_replacing_the_rows_starts_at_the_top() {
        var list = makeList(songs(0, 100));
        list.contentY = 3000;
        list.rows = songs(500, 600);
        wait(150);
        compare(list.count, 100);
        verify(list.contentY <= list.originY + 1, "A different list must open at its start");
    }

    function test_a_shorter_list_is_not_treated_as_an_append() {
        var rows = songs(0, 100);
        var list = makeList(rows);
        list.contentY = 3000;
        list.rows = rows.slice(0, 40);
        wait(150);
        compare(list.count, 40);
        verify(list.contentY <= 40 * 60 + 160, "Rows removed from the tail cannot leave the view past the end");
    }
}
